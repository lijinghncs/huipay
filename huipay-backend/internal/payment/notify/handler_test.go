package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/huipay/huipay-backend/infra/idem"
	"github.com/huipay/huipay-backend/internal/account/ledger"
	"github.com/huipay/huipay-backend/internal/account/repository"
	accountsvc "github.com/huipay/huipay-backend/internal/account/service"
	"github.com/huipay/huipay-backend/internal/domain/vo"
	"github.com/huipay/huipay-backend/internal/order/model"
	orderservice "github.com/huipay/huipay-backend/internal/order/service"
	"github.com/huipay/huipay-backend/internal/payment/channel"
	"github.com/huipay/huipay-backend/internal/payment/router"
)

var memDBSeq uint64

// newMemDSN 返回唯一的共享内存 SQLite DSN，避免跨测试复用同一数据库。
func newMemDSN() string {
	n := atomic.AddUint64(&memDBSeq, 1)
	return fmt.Sprintf("file:mem%d?mode=memory&cache=shared", n)
}

// mockAdapter 可配置的 Adapter，仅用于 VerifyNotify 返回固定载荷。
type mockAdapter struct {
	payload *channel.NotifyPayload
}

func (m *mockAdapter) Code() vo.ChannelCode { return vo.ChannelWeChat }
func (m *mockAdapter) CreatePayment(ctx context.Context, req *channel.CreatePaymentRequest) (*channel.CreatePaymentResponse, error) {
	return &channel.CreatePaymentResponse{}, nil
}
func (m *mockAdapter) QueryPayment(ctx context.Context, channelTradeNo string) (*channel.PaymentStatus, error) {
	return &channel.PaymentStatus{}, nil
}
func (m *mockAdapter) Refund(ctx context.Context, req *channel.RefundRequest) (*channel.RefundResponse, error) {
	return &channel.RefundResponse{}, nil
}
func (m *mockAdapter) Split(ctx context.Context, req *channel.SplitRequest) (*channel.SplitResponse, error) {
	return &channel.SplitResponse{}, nil
}
func (m *mockAdapter) ReturnSplit(ctx context.Context, req *channel.ReturnSplitRequest) error { return nil }
func (m *mockAdapter) FinishSplit(ctx context.Context, req *channel.FinishSplitRequest) error  { return nil }
func (m *mockAdapter) CloseOrder(ctx context.Context, orderNo string) error                    { return nil }
func (m *mockAdapter) VerifyNotify(ctx context.Context, raw []byte, headers map[string]string) (*channel.NotifyPayload, error) {
	return m.payload, nil
}
func (m *mockAdapter) VerifyAndDecrypt(ctx context.Context, raw []byte, headers map[string]string) ([]byte, error) {
	return raw, nil
}

type handlerHB struct {
	db  *gorm.DB
	svc *Handler
}

func (hb *handlerHB) post(t *testing.T, path string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST(path, hb.svc.HandleWechat)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	r.ServeHTTP(w, req)
	return w
}

func createOrder(t *testing.T, db *gorm.DB, no string, amount int64) {
	t.Helper()
	m := &model.OrderModel{OrderNo: no, MerchantID: 1, Amount: amount, Status: string(vo.OrderCreated)}
	if err := db.Create(m).Error; err != nil {
		t.Fatalf("create order: %v", err)
	}
}

func seedSettlement(t *testing.T, db *gorm.DB, balance int64) uint64 {
	t.Helper()
	w := &repository.WalletModel{
		WalletNo: "SETTLE", EntityID: 9001, EntityType: string(vo.EntityPlatform),
		Currency: "CNY", Balance: balance, Status: 1,
	}
	if err := db.Create(w).Error; err != nil {
		t.Fatalf("create settlement wallet: %v", err)
	}
	return w.ID
}

func merchantBalance(t *testing.T, db *gorm.DB, entityID uint64) int64 {
	t.Helper()
	var w repository.WalletModel
	if err := db.Where("entity_id = ?", entityID).First(&w).Error; err != nil {
		t.Fatalf("get merchant wallet: %v", err)
	}
	return w.Balance
}

func countIdem(t *testing.T, db *gorm.DB, scope, key string) int64 {
	t.Helper()
	var n int64
	if err := db.Model(&idem.Record{}).Where("scope = ? AND idempotency_key = ?", scope, key).Count(&n).Error; err != nil {
		t.Fatalf("count idem: %v", err)
	}
	return n
}

func TestHandleWechatSuccessCredit(t *testing.T) {
	db := openMem(t)
	createOrder(t, db, "HP1", 100)
	settle := seedSettlement(t, db, 1000)

	hb := newHandlerOnDB(t, db, &channel.NotifyPayload{
		OrderNo: "HP1", ChannelTradeNo: "txn1", NotifyID: "nid1", Paid: true, PaidAmount: 100,
	}, settle)

	w := hb.post(t, "/v1/notify/wechat", []byte(`{}`))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var order model.OrderModel
	if err := db.Where("order_no = ?", "HP1").First(&order).Error; err != nil {
		t.Fatalf("get order: %v", err)
	}
	if order.Status != string(vo.OrderPaid) {
		t.Fatalf("order status = %s, want PAID", order.Status)
	}
	if got := merchantBalance(t, db, 1); got != 100 {
		t.Fatalf("merchant balance = %d, want 100", got)
	}
	if n := countIdem(t, db, idemScopeNotify, "nid1"); n != 1 {
		t.Fatalf("idem records = %d, want 1", n)
	}
}

func TestHandleWechatIdempotentDuplicate(t *testing.T) {
	db := openMem(t)
	createOrder(t, db, "HP2", 100)
	settle := seedSettlement(t, db, 1000)
	payload := &channel.NotifyPayload{
		OrderNo: "HP2", ChannelTradeNo: "txn2", NotifyID: "nid2", Paid: true, PaidAmount: 100,
	}
	hb := newHandlerOnDB(t, db, payload, settle)

	// 第一次
	w1 := hb.post(t, "/v1/notify/wechat", []byte(`{}`))
	// 第二次（同一 notify_id）
	w2 := hb.post(t, "/v1/notify/wechat", []byte(`{}`))
	if w1.Code != http.StatusOK || w2.Code != http.StatusOK {
		t.Fatalf("codes = %d/%d, want 200/200", w1.Code, w2.Code)
	}
	// 只入账一次
	if got := merchantBalance(t, db, 1); got != 100 {
		t.Fatalf("merchant balance = %d, want 100 (no double credit)", got)
	}
	var n int64
	if err := db.Model(&repository.JournalEntryModel{}).Count(&n).Error; err != nil {
		t.Fatalf("count entries: %v", err)
	}
	if n != 2 {
		t.Fatalf("journal entries = %d, want 2", n)
	}
}

func TestHandleWechatCreditFail(t *testing.T) {
	db := openMem(t)
	createOrder(t, db, "HP3", 100)
	// 结算户不存在 → 入账失败
	hb := newHandlerOnDB(t, db, &channel.NotifyPayload{
		OrderNo: "HP3", ChannelTradeNo: "txn3", NotifyID: "nid3", Paid: true, PaidAmount: 100,
	}, 99999)

	w := hb.post(t, "/v1/notify/wechat", []byte(`{}`))
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
	// 入账失败不写幂等键（让微信重试）
	if n := countIdem(t, db, idemScopeNotify, "nid3"); n != 0 {
		t.Fatalf("idem records = %d, want 0", n)
	}
}

func openMem(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(newMemDSN()), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.OrderModel{}, &repository.WalletModel{}, &repository.JournalEntryModel{}, &idem.Record{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func newHandlerOnDB(t *testing.T, db *gorm.DB, payload *channel.NotifyPayload, settlementWalletID uint64) *handlerHB {
	t.Helper()
	logger := zap.NewNop()
	walletRepo := repository.NewWalletRepo(db)
	journalRepo := repository.NewJournalRepo(db)
	ledgerSvc := ledger.NewService(walletRepo, journalRepo, logger)
	accountSvc := accountsvc.NewService(ledgerSvc, walletRepo, journalRepo, logger)
	orderSvc := orderservice.NewService(db, logger, router.NewDefaultRouter())
	idemStore := idem.NewMySQLStore(db)
	h := New(&mockAdapter{payload: payload}, orderSvc, accountSvc, ledgerSvc, idemStore, settlementWalletID, logger)
	return &handlerHB{db: db, svc: h}
}

var _ = json.Marshal