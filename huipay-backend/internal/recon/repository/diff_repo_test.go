package repository

import (
	"context"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/huipay/huipay-backend/internal/recon/domain"
)

func newTestRepo(t *testing.T) *DiffStore {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&DiffModel{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return NewDiffStore(db)
}

func mustMerchant(v uint64) *uint64 { return &v }

func bizDate(t *testing.T, s string) time.Time {
	t.Helper()
	d, err := time.Parse("2006-01-02", s)
	if err != nil {
		t.Fatalf("parse date: %v", err)
	}
	return d
}

func orderDiff(orderNo string, local, remote int64, detail string) domain.Diff {
	return domain.Diff{
		OrderNo:      orderNo,
		LocalAmount:  domain.Int64(local),
		RemoteAmount: domain.Int64(remote),
		Detail:       detail,
	}
}

// TestWriteOrderDiffsIdempotent 同商户+业务日+类型重写：清未核销旧差异、保留已核销与其他类型（🔴1 修复回归）。
func TestWriteOrderDiffsIdempotent(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	d := bizDate(t, "2026-08-01")
	mid := mustMerchant(1)

	if _, err := repo.WriteOrderDiffs(ctx, mid, d, domain.DiffTypeLong, []domain.Diff{
		orderDiff("O1", 100, 0, "local only"),
		orderDiff("O2", 200, 0, "local only"),
	}); err != nil {
		t.Fatalf("first write: %v", err)
	}
	// 核销 O1
	items, total, err := repo.ListForMerchant(ctx, 1, "", nil, time.Time{}, time.Time{}, 0, 10)
	if err != nil || total != 2 || len(items) != 2 {
		t.Fatalf("list: total=%d items=%d err=%v", total, len(items), err)
	}
	var o1ID uint64
	for _, it := range items {
		if it.OrderNo != nil && *it.OrderNo == "O1" {
			o1ID = it.ID
		}
	}
	if o1ID == 0 {
		t.Fatal("O1 not found")
	}
	if _, err := repo.Resolve(ctx, 1, o1ID); err != nil {
		t.Fatalf("resolve: %v", err)
	}

	// 重写：仅 1 条新 LONG
	n, err := repo.WriteOrderDiffs(ctx, mid, d, domain.DiffTypeLong, []domain.Diff{orderDiff("O3", 300, 0, "local only")})
	if err != nil || n != 1 {
		t.Fatalf("rewrite: n=%d err=%v", n, err)
	}

	items, total, err = repo.ListForMerchant(ctx, 1, "", nil, time.Time{}, time.Time{}, 0, 10)
	if err != nil || total != 2 {
		t.Fatalf("after rewrite: total=%d err=%v (want 2: resolved O1 + new O3)", total, err)
	}
	seen := map[string]bool{}
	for _, it := range items {
		if it.OrderNo != nil {
			seen[*it.OrderNo] = true
		}
	}
	if !seen["O1"] || !seen["O3"] || seen["O2"] {
		t.Fatalf("unexpected rows after rewrite: %v", seen)
	}
}

// TestWriteOrderDiffsDoesNotTouchOtherTypes 写 LONG 不清理同日 SPLIT_POST 差异（渠道对账误删缺陷回归）。
func TestWriteOrderDiffsDoesNotTouchOtherTypes(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	d := bizDate(t, "2026-08-01")
	mid := mustMerchant(1)

	if _, err := repo.WriteOrderDiffs(ctx, mid, d, domain.DiffTypeSplitPost, []domain.Diff{orderDiff("S1", 100, 90, "{}")}); err != nil {
		t.Fatalf("write post: %v", err)
	}
	if _, err := repo.WriteOrderDiffs(ctx, mid, d, domain.DiffTypeLong, []domain.Diff{orderDiff("L1", 100, 0, "local only")}); err != nil {
		t.Fatalf("write long: %v", err)
	}

	_, total, err := repo.ListByMerchantAndType(ctx, 1, domain.DiffTypeSplitPost, nil, time.Time{}, time.Time{}, 0, 10)
	if err != nil || total != 1 {
		t.Fatalf("SPLIT_POST rows after LONG write: total=%d err=%v (want 1)", total, err)
	}
}

// TestWriteOrderDiffsNilMerchant SHORT 无法归属商户时以 NULL 商户落库。
func TestWriteOrderDiffsNilMerchant(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	d := bizDate(t, "2026-08-01")

	n, err := repo.WriteOrderDiffs(ctx, nil, d, domain.DiffTypeShort, []domain.Diff{orderDiff("X1", 0, 50, "channel only")})
	if err != nil || n != 1 {
		t.Fatalf("write: n=%d err=%v", n, err)
	}
	var m DiffModel
	if err := repo.db.Where("diff_type = ?", domain.DiffTypeShort).First(&m).Error; err != nil {
		t.Fatalf("query: %v", err)
	}
	if m.MerchantID != nil {
		t.Fatalf("merchant_id = %v, want nil", *m.MerchantID)
	}
}

// TestWritePrecheck 前置对账差异：单行汇总 + 区间内同类型重写幂等。
func TestWritePrecheck(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	start := bizDate(t, "2026-08-01")
	end := bizDate(t, "2026-08-03")

	id1, err := repo.WritePrecheck(ctx, 1, start, end, domain.DiffTypeSplitTotal, `{"diff":10}`)
	if err != nil || id1 == 0 {
		t.Fatalf("write: id=%d err=%v", id1, err)
	}
	id2, err := repo.WritePrecheck(ctx, 1, start, end, domain.DiffTypeSplitTotal, `{"diff":20}`)
	if err != nil {
		t.Fatalf("rewrite: %v", err)
	}

	_, total, err := repo.ListByMerchantAndType(ctx, 1, domain.DiffTypeSplitTotal, nil, time.Time{}, time.Time{}, 0, 10)
	if err != nil || total != 1 {
		t.Fatalf("total=%d err=%v (want 1 after rewrite)", total, err)
	}
	m, err := repo.GetByID(ctx, id2, 1)
	if err != nil || m == nil {
		t.Fatalf("GetByID: %v", err)
	}
	if m.Detail != `{"diff":20}` {
		t.Fatalf("detail = %q, want latest", m.Detail)
	}
}

// TestResolveByID 管理端核销不限商户。
func TestResolveByID(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	d := bizDate(t, "2026-08-01")
	if _, err := repo.WriteOrderDiffs(ctx, mustMerchant(7), d, domain.DiffTypeMismatch, []domain.Diff{orderDiff("M1", 100, 90, "amount mismatch")}); err != nil {
		t.Fatalf("write: %v", err)
	}
	items, _, _ := repo.ListByMerchantAndType(ctx, 7, domain.DiffTypeMismatch, nil, time.Time{}, time.Time{}, 0, 10)
	if len(items) != 1 {
		t.Fatalf("items = %d", len(items))
	}
	if _, err := repo.ResolveByID(ctx, items[0].ID); err != nil {
		t.Fatalf("ResolveByID: %v", err)
	}
	unresolved := false
	_, total, _ := repo.ListByMerchantAndType(ctx, 7, domain.DiffTypeMismatch, &unresolved, time.Time{}, time.Time{}, 0, 10)
	if total != 0 {
		t.Fatalf("unresolved total = %d, want 0", total)
	}
}
