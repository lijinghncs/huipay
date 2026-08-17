package alloc

import (
	"math"
	"testing"
	"time"

	"github.com/huipay/huipay-backend/internal/split/executor"
	"github.com/huipay/huipay-backend/internal/split/rule"
)

func TestCompute_Proportional(t *testing.T) {
	// 100 元按 30:70 比例分配
	in := Input{
		Rule: &rule.Rule{
			Allocations: []rule.Allocation{
				{ReceiverEntityID: 1, ReceiverType: "STORE", RatioBps: 3000},
				{ReceiverEntityID: 2, ReceiverType: "STORE", RatioBps: 7000},
			},
		},
		Total: 10000,
	}
	items, err := Compute(in)
	if err != nil {
		t.Fatalf("Compute() error = %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("len(items) = %d, want 2", len(items))
	}
	if items[0].Amount+items[1].Amount != 10000 {
		t.Errorf("sum = %d, want 10000", items[0].Amount+items[1].Amount)
	}
}

func TestCompute_FixedAmount(t *testing.T) {
	// 固定金额 50 元 + 比例 50%
	in := Input{
		Rule: &rule.Rule{
			Allocations: []rule.Allocation{
				{ReceiverEntityID: 1, ReceiverType: "STORE", FixedAmount: 5000},
				{ReceiverEntityID: 2, ReceiverType: "STORE", RatioBps: 5000},
			},
		},
		Total: 10000,
	}
	items, err := Compute(in)
	if err != nil {
		t.Fatalf("Compute() error = %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("len(items) = %d, want 2", len(items))
	}
	if items[0].Amount != 5000 {
		t.Errorf("fixed amount = %d, want 5000", items[0].Amount)
	}
	if items[1].Amount != 5000 {
		t.Errorf("ratio amount = %d, want 5000", items[1].Amount)
	}
}

func TestCompute_ExceedTotal(t *testing.T) {
	// 固定金额超出总额 → 拒付
	in := Input{
		Rule: &rule.Rule{
			Allocations: []rule.Allocation{
				{ReceiverEntityID: 1, ReceiverType: "STORE", FixedAmount: 15000},
			},
		},
		Total: 10000,
	}
	_, err := Compute(in)
	if err == nil {
		t.Fatal("Compute() expected error for exceed total")
	}
}

func TestCompute_LastComplement(t *testing.T) {
	// 1/3 分账，末笔补齐（100 / 3 ≈ 33，末笔 = 100-33-33 = 34）
	in := Input{
		Rule: &rule.Rule{
			Allocations: []rule.Allocation{
				{ReceiverEntityID: 1, ReceiverType: "STORE", RatioBps: 3333},
				{ReceiverEntityID: 2, ReceiverType: "STORE", RatioBps: 3333},
				{ReceiverEntityID: 3, ReceiverType: "STORE", RatioBps: 3334},
			},
		},
		Total: 100,
	}
	items, err := Compute(in)
	if err != nil {
		t.Fatalf("Compute() error = %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("len(items) = %d, want 3", len(items))
	}
	sum := items[0].Amount + items[1].Amount + items[2].Amount
	if sum != 100 {
		t.Errorf("sum = %d, want 100", sum)
	}
}

func TestCompute_ZeroAmount(t *testing.T) {
	in := Input{
		Rule: &rule.Rule{
			Allocations: []rule.Allocation{
				{ReceiverEntityID: 1, ReceiverType: "STORE", RatioBps: 10000},
			},
		},
		Total: 0,
	}
	_, err := Compute(in)
	if err == nil {
		t.Fatal("Compute() expected error for zero total (amount <= 0)")
	}
}

func TestCompute_EmptyAllocs(t *testing.T) {
	in := Input{
		Rule: &rule.Rule{
			Allocations: []rule.Allocation{},
		},
		Total: 10000,
	}
	_, err := Compute(in)
	if err == nil {
		t.Fatal("Compute() expected error for empty allocs")
	}
}

func TestCompute_NilRule(t *testing.T) {
	in := Input{
		Rule:  nil,
		Total: 10000,
	}
	_, err := Compute(in)
	if err == nil {
		t.Fatal("Compute() expected error for nil rule")
	}
}

func TestParsePaidAt(t *testing.T) {
	tests := []struct {
		name string
		s    string
		want time.Time
	}{
		{"with time", "2024-01-15T10:30:00+08:00", time.Date(2024, 1, 15, 10, 30, 0, 0, time.FixedZone("CST", 8*3600))},
		{"empty", "", time.Time{}},
		{"invalid", "not-a-date", time.Time{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParsePaidAt(tt.s)
			if !got.Equal(tt.want) {
				t.Errorf("ParsePaidAt(%q) = %v, want %v", tt.s, got, tt.want)
			}
		})
	}
}

func TestCollectBizDates(t *testing.T) {
	tests := []struct {
		name  string
		start time.Time
		end   time.Time
		want  int
	}{
		{"single day", time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), 0},
		{"three days", time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2024, 1, 4, 0, 0, 0, 0, time.UTC), 3},
		{"month span", time.Date(2024, 1, 31, 0, 0, 0, 0, time.UTC), time.Date(2024, 2, 2, 0, 0, 0, 0, time.UTC), 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CollectBizDates(tt.start, tt.end)
			if len(got) != tt.want {
				t.Errorf("CollectBizDates() len = %d, want %d", len(got), tt.want)
			}
		})
	}
}

func TestAllocationsToItems(t *testing.T) {
	allocs := []executor.Allocation{
		{EntityID: 1, EntityType: "STORE", Amount: 5000},
	}
	items := AllocationsToItems(allocs)
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}
	if items[0].ReceiverEntityID != 1 {
		t.Errorf("ReceiverEntityID = %d, want 1", items[0].ReceiverEntityID)
	}
	if items[0].Amount != 5000 {
		t.Errorf("Amount = %d, want 5000", items[0].Amount)
	}
}

func TestItemsToAllocations(t *testing.T) {
	items := []BillItem{
		{ReceiverEntityID: 1, ReceiverType: "STORE", Amount: 5000},
	}
	allocs := ItemsToAllocations(items)
	if len(allocs) != 1 {
		t.Fatalf("len(allocs) = %d, want 1", len(allocs))
	}
	if allocs[0].EntityID != 1 {
		t.Errorf("EntityID = %d, want 1", allocs[0].EntityID)
	}
	if allocs[0].Amount != 5000 {
		t.Errorf("Amount = %d, want 5000", allocs[0].Amount)
	}
}

func TestTruncateMsg(t *testing.T) {
	tests := []struct {
		name string
		s    string
		want int
	}{
		{"short", "hello", 5},
		{"exact", string(make([]byte, 1000)), 1000},
		{"long", string(make([]byte, 2000)), 1000},
		{"empty", "", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TruncateMsg(tt.s)
			if len(got) != tt.want {
				t.Errorf("TruncateMsg() len = %d, want %d", len(got), tt.want)
			}
		})
	}
}

func TestFormatBatchNo(t *testing.T) {
	start := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)
	end := time.Date(2024, 1, 16, 0, 0, 0, 0, time.UTC)
	got := FormatBatchNo(100, start, end)
	if got == "" {
		t.Error("FormatBatchNo() returned empty string")
	}
}

func TestCompute_EdgeCases(t *testing.T) {
	// 1 分钱按 50:50 分账
	in := Input{
		Rule: &rule.Rule{
			Allocations: []rule.Allocation{
				{ReceiverEntityID: 1, ReceiverType: "STORE", RatioBps: 5000},
				{ReceiverEntityID: 2, ReceiverType: "STORE", RatioBps: 5000},
			},
		},
		Total: 1,
	}
	_, err := Compute(in)
	if err == nil {
		t.Fatal("Compute(1) should fail: 1 cent cannot be split 50:50 (integer division yields 0)")
	}
}

func TestCompute_AllRatioBps(t *testing.T) {
	// 占比总和 = 10000 (100%)
	in := Input{
		Rule: &rule.Rule{
			Allocations: []rule.Allocation{
				{ReceiverEntityID: 1, ReceiverType: "STORE", RatioBps: 10000},
			},
		},
		Total: 5000,
	}
	items, err := Compute(in)
	if err != nil {
		t.Fatalf("Compute() error = %v", err)
	}
	if len(items) != 1 || items[0].Amount != 5000 {
		t.Errorf("amount = %d, want 5000", items[0].Amount)
	}
}

func TestCompute_LargeAmount(t *testing.T) {
	// 大额分账，验证精度不丢失
	in := Input{
		Rule: &rule.Rule{
			Allocations: []rule.Allocation{
				{ReceiverEntityID: 1, ReceiverType: "STORE", RatioBps: 3333},
				{ReceiverEntityID: 2, ReceiverType: "STORE", RatioBps: 3333},
				{ReceiverEntityID: 3, ReceiverType: "STORE", RatioBps: 3334},
			},
		},
		Total: 100000000, // 100 万元
	}
	items, err := Compute(in)
	if err != nil {
		t.Fatalf("Compute() error = %v", err)
	}
	sum := int64(0)
	for _, it := range items {
		sum += it.Amount
	}
	if sum != 100000000 {
		t.Errorf("sum = %d, want 100000000", sum)
	}
}

func TestCompute_ExactDivision(t *testing.T) {
	// 100 元分给 4 个人各 25%
	in := Input{
		Rule: &rule.Rule{
			Allocations: []rule.Allocation{
				{ReceiverEntityID: 1, ReceiverType: "STORE", RatioBps: 2500},
				{ReceiverEntityID: 2, ReceiverType: "STORE", RatioBps: 2500},
				{ReceiverEntityID: 3, ReceiverType: "STORE", RatioBps: 2500},
				{ReceiverEntityID: 4, ReceiverType: "STORE", RatioBps: 2500},
			},
		},
		Total: 10000,
	}
	items, err := Compute(in)
	if err != nil {
		t.Fatalf("Compute() error = %v", err)
	}
	for _, it := range items {
		if it.Amount != 2500 {
			t.Errorf("each amount = %d, want 2500", it.Amount)
		}
	}
}

func TestCompute_RatioOverflow(t *testing.T) {
	in := Input{
		Rule: &rule.Rule{
			Allocations: []rule.Allocation{
				{ReceiverEntityID: 1, ReceiverType: "STORE", RatioBps: math.MaxInt64},
			},
		},
		Total: 10000,
	}
	_, err := Compute(in)
	if err != nil {
		t.Logf("Compute() with overflow ratio error = %v (expected depending on overflow behavior)", err)
	}
}

func TestCompute_PartialRatio(t *testing.T) {
	// 部分分账（合计 < 10000），不触发末笔补齐
	in := Input{
		Rule: &rule.Rule{
			Allocations: []rule.Allocation{
				{ReceiverEntityID: 1, ReceiverType: "STORE", RatioBps: 3000},
			},
		},
		Total: 10000,
	}
	items, err := Compute(in)
	if err != nil {
		t.Fatalf("Compute() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}
	if items[0].Amount != 3000 {
		t.Errorf("amount = %d, want 3000", items[0].Amount)
	}
}

func TestCompute_AllStoresExpand(t *testing.T) {
	// ALL_STORES 展开
	in := Input{
		Rule: &rule.Rule{
			Allocations: []rule.Allocation{
				{ReceiverEntityID: 1, ReceiverType: "STORE", ReceiverScope: "ALL_STORES", RatioBps: 10000},
			},
		},
		Total: 6000,
		StoreRevenues: []StoreRevenue{
			{StoreID: 1, Paid: 100},
			{StoreID: 2, Paid: 200},
		},
	}
	items, err := Compute(in)
	if err != nil {
		t.Fatalf("Compute() error = %v", err)
	}
	// 6000 按 1:2 分给门店 1 和 2
	if len(items) != 2 {
		t.Fatalf("len(items) = %d, want 2", len(items))
	}
	if items[0].Amount+items[1].Amount != 6000 {
		t.Errorf("sum = %d, want 6000", items[0].Amount+items[1].Amount)
	}
}