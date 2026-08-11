package scheduler

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/huipay/huipay-backend/internal/domain/vo"
	"github.com/huipay/huipay-backend/internal/order/model"
	"github.com/huipay/huipay-backend/internal/payment/channel"
	"github.com/huipay/huipay-backend/internal/payment/router"
)

var memDBSeq int64

// newMemDSN 返回唯一的共享内存 SQLite DSN，避免跨测试复用同一数据库。
func newMemDSN() string {
	n := atomic.AddInt64(&memDBSeq, 1)
	return fmt.Sprintf("file:mem%d?mode=memory&cache=shared", n)
}

// mockAdapter 记录 CloseOrder 调用，可注入 panic。
type mockAdapter struct {
	code      vo.ChannelCode
	mu        sync.Mutex
	closed    []string
	panicCall bool
}

func (m *mockAdapter) Code() vo.ChannelCode { return m.code }

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
func (m *mockAdapter) VerifyNotify(ctx context.Context, raw []byte, headers map[string]string) (*channel.NotifyPayload, error) {
	return &channel.NotifyPayload{}, nil
}
func (m *mockAdapter) VerifyAndDecrypt(ctx context.Context, raw []byte, headers map[string]string) ([]byte, error) {
	return raw, nil
}
func (m *mockAdapter) CloseOrder(ctx context.Context, orderNo string) error {
	if m.panicCall {
		panic("boom")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = append(m.closed, orderNo)
	return nil
}

func (m *mockAdapter) closedSet() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := append([]string{}, m.closed...)
	return out
}

func newSchedulerTest(ctx context.Context, t *testing.T) (*gorm.DB, *mockAdapter, *CloseExpiredScheduler) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(newMemDSN()), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.OrderModel{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	adapter := &mockAdapter{code: vo.ChannelWeChat}
	rt := router.NewDefaultRouter()
	rt.Register(adapter)
	s := NewCloseExpiredScheduler(db, rt, nil, time.Second, zap.NewNop())
	return db, adapter, s
}

func insertOrder(t *testing.T, db *gorm.DB, no string, status string, channelCode vo.ChannelCode, expireAt *time.Time) {
	t.Helper()
	m := &model.OrderModel{
		OrderNo: no, Status: status, Channel: channelCode, ExpireAt: expireAt,
	}
	if err := db.Create(m).Error; err != nil {
		t.Fatalf("insert order: %v", err)
	}
}

func orderStatus(t *testing.T, db *gorm.DB, no string) string {
	t.Helper()
	var m model.OrderModel
	if err := db.Where("order_no = ?", no).First(&m).Error; err != nil {
		t.Fatalf("get order: %v", err)
	}
	return m.Status
}

func TestCloseExpiredExpiredOrder(t *testing.T) {
	ctx := context.Background()
	db, adapter, s := newSchedulerTest(ctx, t)
	past := time.Now().Add(-time.Minute)
	insertOrder(t, db, "HP1", string(vo.OrderCreated), vo.ChannelWeChat, &past)

	s.runOnce(ctx)

	if got := orderStatus(t, db, "HP1"); got != string(vo.OrderClosed) {
		t.Fatalf("status = %s, want CLOSED", got)
	}
	if closed := adapter.closedSet(); len(closed) != 1 || closed[0] != "HP1" {
		t.Fatalf("CloseOrder called = %v, want [HP1]", closed)
	}
}

func TestCloseExpiredUnexpired(t *testing.T) {
	ctx := context.Background()
	db, adapter, s := newSchedulerTest(ctx, t)
	future := time.Now().Add(time.Hour)
	insertOrder(t, db, "HP2", string(vo.OrderCreated), vo.ChannelWeChat, &future)

	s.runOnce(ctx)

	if got := orderStatus(t, db, "HP2"); got != string(vo.OrderCreated) {
		t.Fatalf("status = %s, want CREATED", got)
	}
	if closed := adapter.closedSet(); len(closed) != 0 {
		t.Fatalf("CloseOrder called = %v, want none", closed)
	}
}

func TestCloseExpiredEmptyChannel(t *testing.T) {
	ctx := context.Background()
	db, adapter, s := newSchedulerTest(ctx, t)
	past := time.Now().Add(-time.Minute)
	insertOrder(t, db, "HP3", string(vo.OrderCreated), "", &past)

	s.runOnce(ctx)

	if got := orderStatus(t, db, "HP3"); got != string(vo.OrderClosed) {
		t.Fatalf("status = %s, want CLOSED", got)
	}
	if closed := adapter.closedSet(); len(closed) != 0 {
		t.Fatalf("CloseOrder called = %v, want none (empty channel)", closed)
	}
}

func TestCloseExpiredCASConflict(t *testing.T) {
	ctx := context.Background()
	db, _, s := newSchedulerTest(ctx, t)
	past := time.Now().Add(-time.Minute)
	insertOrder(t, db, "HP4", string(vo.OrderPaid), vo.ChannelWeChat, &past)

	s.runOnce(ctx)

	// 已 PAID 订单不应被关单
	if got := orderStatus(t, db, "HP4"); got != string(vo.OrderPaid) {
		t.Fatalf("status = %s, want PAID", got)
	}
}

func TestCloseExpiredPanicDoesNotKillScheduler(t *testing.T) {
	ctx := context.Background()
	db, adapter, s := newSchedulerTest(ctx, t)
	past := time.Now().Add(-time.Minute)
	insertOrder(t, db, "HP5", string(vo.OrderCreated), vo.ChannelWeChat, &past)
	adapter.panicCall = true

	// runOnce 内部 recover，panic 不会泄漏到调用方
	s.runOnce(ctx)

	// 关单调用 panic 后，该订单本次未完成 DB 关单（循环被中断）
	if got := orderStatus(t, db, "HP5"); got != string(vo.OrderCreated) {
		t.Fatalf("status = %s, want CREATED (panic aborted this tick)", got)
	}

	// 不代表调度挂掉：恢复正常后，下一次 tick 仍能关单
	adapter.panicCall = false
	s.runOnce(ctx)
	if got := orderStatus(t, db, "HP5"); got != string(vo.OrderClosed) {
		t.Fatalf("status after retry = %s, want CLOSED", got)
	}
}
