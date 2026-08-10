package ledger

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"

	"go.uber.org/zap"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/huipay/huipay-backend/internal/account/repository"
	"github.com/huipay/huipay-backend/internal/domain/vo"
)

var memDBSeq int64

// newMemDSN 返回唯一的共享内存 SQLite DSN，避免跨测试复用同一数据库。
func newMemDSN() string {
	n := atomic.AddInt64(&memDBSeq, 1)
	return fmt.Sprintf("file:mem%d?mode=memory&cache=shared", n)
}

func newLedgerTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(newMemDSN()), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&repository.WalletModel{}, &repository.JournalEntryModel{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func newLedgerService(db *gorm.DB) *Service {
	return NewService(repository.NewWalletRepo(db), repository.NewJournalRepo(db), zap.NewNop())
}

// seedSettlementWallet 创建带指定余额的结算钱包。
func seedSettlementWallet(t *testing.T, db *gorm.DB, balance int64) uint64 {
	t.Helper()
	w := &repository.WalletModel{
		WalletNo: "SETTLE1", EntityID: 9001, EntityType: string(vo.EntityPlatform),
		Currency: "CNY", Balance: balance, Status: 1,
	}
	if err := db.Create(w).Error; err != nil {
		t.Fatalf("create settlement wallet: %v", err)
	}
	return w.ID
}

func countEntries(t *testing.T, db *gorm.DB) int64 {
	t.Helper()
	var n int64
	if err := db.Model(&repository.JournalEntryModel{}).Count(&n).Error; err != nil {
		t.Fatalf("count entries: %v", err)
	}
	return n
}

func getBalance(t *testing.T, db *gorm.DB, id uint64) int64 {
	t.Helper()
	var w repository.WalletModel
	if err := db.First(&w, id).Error; err != nil {
		t.Fatalf("get wallet %d: %v", id, err)
	}
	return w.Balance
}

func TestCreditFromSettlementSuccess(t *testing.T) {
	db := newLedgerTestDB(t)
	svc := newLedgerService(db)
	settleID := seedSettlementWallet(t, db, 1000)

	err := svc.CreditFromSettlement(context.Background(), &CreditFromSettlementRequest{
		SettlementWalletID: settleID,
		ToEntityID:         1,
		ToEntityType:       vo.EntityMerchant,
		Amount:             100,
		BizType:            "PAYMENT",
		BizID:              "HP1",
	})
	if err != nil {
		t.Fatalf("credit: %v", err)
	}

	if got := getBalance(t, db, settleID); got != 900 {
		t.Fatalf("settle balance = %d, want 900", got)
	}
	// 商户钱包被自动创建并入账
	var merchant repository.WalletModel
	if err := db.Where("entity_id = ?", 1).First(&merchant).Error; err != nil {
		t.Fatalf("merchant wallet not created: %v", err)
	}
	if merchant.Balance != 100 {
		t.Fatalf("merchant balance = %d, want 100", merchant.Balance)
	}
	if n := countEntries(t, db); n != 2 {
		t.Fatalf("entries = %d, want 2 (DEBIT settle + CREDIT merchant)", n)
	}
}

func TestCreditFromSettlementInsufficient(t *testing.T) {
	db := newLedgerTestDB(t)
	svc := newLedgerService(db)
	settleID := seedSettlementWallet(t, db, 50)

	err := svc.CreditFromSettlement(context.Background(), &CreditFromSettlementRequest{
		SettlementWalletID: settleID,
		ToEntityID:         1,
		ToEntityType:       vo.EntityMerchant,
		Amount:             100,
		BizType:            "PAYMENT",
		BizID:              "HP2",
	})
	if err == nil {
		t.Fatal("expected error for insufficient balance")
	}
	if got := getBalance(t, db, settleID); got != 50 {
		t.Fatalf("settle balance = %d, want unchanged 50", got)
	}
	if n := countEntries(t, db); n != 0 {
		t.Fatalf("entries = %d, want 0", n)
	}
}

func TestCreditFromSettlementIdempotent(t *testing.T) {
	db := newLedgerTestDB(t)
	svc := newLedgerService(db)
	settleID := seedSettlementWallet(t, db, 1000)

	req := &CreditFromSettlementRequest{
		SettlementWalletID: settleID,
		ToEntityID:         1,
		ToEntityType:       vo.EntityMerchant,
		Amount:             100,
		BizType:            "PAYMENT",
		BizID:              "HP3",
	}
	if err := svc.CreditFromSettlement(context.Background(), req); err != nil {
		t.Fatalf("first credit: %v", err)
	}
	// 相同幂等键重复调用 → 数据库唯一键冲突，流水不重复
	if err := svc.CreditFromSettlement(context.Background(), req); err == nil {
		t.Fatal("expected error on duplicate idempotency key")
	}
	if n := countEntries(t, db); n != 2 {
		t.Fatalf("entries = %d, want still 2", n)
	}
	// 余额不变（事务回滚）
	if got := getBalance(t, db, settleID); got != 900 {
		t.Fatalf("settle balance = %d, want 900", got)
	}
}