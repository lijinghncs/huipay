package reconcile

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/huipay/huipay-backend/internal/domain/vo"
	"github.com/huipay/huipay-backend/internal/order/model"
)

var memDBSeq uint64

// newMemDB 返回唯一的共享内存 SQLite DSN，避免跨测试复用同一数据库。
func newMemDB(t *testing.T) *gorm.DB {
	t.Helper()
	n := atomic.AddUint64(&memDBSeq, 1)
	dsn := fmt.Sprintf("file:recmem%d?mode=memory&cache=shared", n)
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open mem db: %v", err)
	}
	if err := db.AutoMigrate(&model.OrderModel{}, &DiffModel{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(func() {
		sql, _ := db.DB()
		_ = sql.Close()
	})
	return db
}

// seedPaidOrder 插入一笔已支付本地订单。
func seedPaidOrder(t *testing.T, db *gorm.DB, orderNo, txnID string, amount int64) {
	t.Helper()
	paidAt := time.Date(2026, 8, 10, 12, 0, 0, 0, time.Local)
	o := model.OrderModel{
		OrderNo:        orderNo,
		MerchantOrderNo: orderNo,
		MerchantID:     10001,
		Amount:         amount,
		PaidAmount:     amount,
		Channel:        vo.ChannelWeChat,
		ChannelTradeNo: txnID,
		Status:         string(vo.OrderPaid),
		PaidAt:         &paidAt,
	}
	if err := db.Create(&o).Error; err != nil {
		t.Fatalf("seed order: %v", err)
	}
}

// TestReconcileNormal 本地 = 微信，差异为空。
func TestReconcileNormal(t *testing.T) {
	db := newMemDB(t)
	seedPaidOrder(t, db, "HP0001", "TXN1001", 100)

	d, _ := newMockDownloader(t, gzipCSV([]string{
		billHeader,
		billLine("2026-08-10 10:00:00", "TXN1001", "HP0001", "100"),
	}))
	report, err := Reconcile(context.Background(), d, db, "2026-08-10")
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if len(report.LongOrders) != 0 || len(report.ShortOrders) != 0 || len(report.MismatchOrders) != 0 {
		t.Fatalf("expected no differences, got %+v", report)
	}
}

// TestReconcileLong 本地有 / 微信无 → LONG。
func TestReconcileLong(t *testing.T) {
	db := newMemDB(t)
	seedPaidOrder(t, db, "HP0001", "TXN1001", 100)

	d, _ := newMockDownloader(t, gzipCSV([]string{billHeader})) // 微信无任何交易
	report, err := Reconcile(context.Background(), d, db, "2026-08-10")
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if len(report.LongOrders) != 1 || report.LongOrders[0].OrderNo != "HP0001" {
		t.Fatalf("expected 1 LONG for HP0001, got %+v", report.LongOrders)
	}
}

// TestReconcileShort 微信有 / 本地无 → SHORT。
func TestReconcileShort(t *testing.T) {
	db := newMemDB(t)

	d, _ := newMockDownloader(t, gzipCSV([]string{
		billHeader,
		billLine("2026-08-10 10:00:00", "TXN9999", "HP9999", "50"),
	}))
	report, err := Reconcile(context.Background(), d, db, "2026-08-10")
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if len(report.ShortOrders) != 1 || report.ShortOrders[0].OrderNo != "HP9999" {
		t.Fatalf("expected 1 SHORT for HP9999, got %+v", report.ShortOrders)
	}
}

// TestReconcileMismatch 金额不一致 → MISMATCH。
func TestReconcileMismatch(t *testing.T) {
	db := newMemDB(t)
	seedPaidOrder(t, db, "HP0001", "TXN1001", 100)

	d, _ := newMockDownloader(t, gzipCSV([]string{
		billHeader,
		billLine("2026-08-10 10:00:00", "TXN1001", "HP0001", "80"), // 微信金额 80 ≠ 本地 100
	}))
	report, err := Reconcile(context.Background(), d, db, "2026-08-10")
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if len(report.MismatchOrders) != 1 || report.MismatchOrders[0].OrderNo != "HP0001" {
		t.Fatalf("expected 1 MISMATCH for HP0001, got %+v", report.MismatchOrders)
	}
	if report.MismatchOrders[0].LocalAmount != 100 || report.MismatchOrders[0].ChannelAmount != 80 {
		t.Fatalf("amount mismatch not captured: %+v", report.MismatchOrders[0])
	}
}

// TestSaveDiffs 差异落库且幂等（重复执行先清空再写）。
func TestSaveDiffs(t *testing.T) {
	db := newMemDB(t)
	report := &ReconcileReport{
		BizDate: "2026-08-10",
		LongOrders: []DiffEntry{{OrderNo: "HP0001", TransactionID: "TXN1001", LocalAmount: 100, Detail: "local only"}},
		ShortOrders: []DiffEntry{{OrderNo: "HP9999", TransactionID: "TXN9999", ChannelAmount: 50, Detail: "channel only"}},
	}
	if err := SaveDiffs(context.Background(), db, report); err != nil {
		t.Fatalf("save diffs: %v", err)
	}
	var count int64
	if err := db.Model(&DiffModel{}).Where("biz_date = ?", "2026-08-10").Count(&count).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 2 {
		t.Fatalf("count = %d, want 2", count)
	}

	// 再次执行：先清空再写，仍为 2 条
	if err := SaveDiffs(context.Background(), db, report); err != nil {
		t.Fatalf("re-save diffs: %v", err)
	}
	if err := db.Model(&DiffModel{}).Where("biz_date = ?", "2026-08-10").Count(&count).Error; err != nil {
		t.Fatalf("re-count: %v", err)
	}
	if count != 2 {
		t.Fatalf("after re-save count = %d, want 2 (idempotent)", count)
	}
}

// TestSaveDiffsNilReport nil 报告为空操作。
func TestSaveDiffsNilReport(t *testing.T) {
	db := newMemDB(t)
	if err := SaveDiffs(context.Background(), db, nil); err != nil {
		t.Fatalf("save nil report: %v", err)
	}
}