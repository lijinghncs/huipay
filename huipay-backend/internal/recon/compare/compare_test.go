package compare

import (
	"testing"

	"github.com/huipay/huipay-backend/internal/recon/domain"
)

// TestTotals 总额比对返回差额。
func TestTotals(t *testing.T) {
	if got := Totals(100, 100); got != 0 {
		t.Fatalf("Totals(100,100) = %d, want 0", got)
	}
	if got := Totals(120, 100); got != 20 {
		t.Fatalf("Totals(120,100) = %d, want 20", got)
	}
}

// TestRows 按 key 比对：缺失/多出/金额不一致均产出差异行。
func TestRows(t *testing.T) {
	local := map[string]int64{"a": 100, "b": 200, "c": 300}
	remote := map[string]int64{"a": 100, "b": 150, "d": 400}

	diffs := Rows(local, remote)
	if len(diffs) != 3 {
		t.Fatalf("len(diffs) = %d, want 3", len(diffs))
	}
	got := map[string]RowDiff[string]{}
	for _, d := range diffs {
		got[d.Key] = d
	}
	if d, ok := got["b"]; !ok || d.LocalAmount != 200 || d.RemoteAmount != 150 {
		t.Fatalf("key b mismatch diff wrong: %+v", got["b"])
	}
	if d, ok := got["c"]; !ok || d.LocalAmount != 300 || d.RemoteAmount != 0 {
		t.Fatalf("key c local-only diff wrong: %+v", got["c"])
	}
	if d, ok := got["d"]; !ok || d.LocalAmount != 0 || d.RemoteAmount != 400 {
		t.Fatalf("key d remote-only diff wrong: %+v", got["d"])
	}
}

// TestMatchBillsNormal 本地 = 账单，无差异。
func TestMatchBillsNormal(t *testing.T) {
	local := []domain.LocalOrder{{OrderNo: "HP0001", ChannelTradeNo: "TXN1001", PaidAmount: 100}}
	bill := []domain.BillEntry{{TransactionID: "TXN1001", OutTradeNo: "HP0001", OrderAmount: 100}}

	rep := MatchBills(local, bill)
	if len(rep.Long) != 0 || len(rep.Short) != 0 || len(rep.Mismatch) != 0 {
		t.Fatalf("expected no differences, got %+v", rep)
	}
}

// TestMatchBillsLong 本地有 / 渠道无 → LONG。
func TestMatchBillsLong(t *testing.T) {
	local := []domain.LocalOrder{{OrderNo: "HP0001", ChannelTradeNo: "TXN1001", PaidAmount: 100, MerchantID: 10001}}

	rep := MatchBills(local, nil)
	if len(rep.Long) != 1 || rep.Long[0].OrderNo != "HP0001" || rep.Long[0].Local() != 100 || rep.Long[0].Reason != domain.ReasonLocalOnly {
		t.Fatalf("expected 1 LONG for HP0001, got %+v", rep.Long)
	}
	if len(rep.Short) != 0 || len(rep.Mismatch) != 0 {
		t.Fatalf("unexpected short/mismatch: %+v", rep)
	}
}

// TestMatchBillsShort 渠道有 / 本地无 → SHORT。
func TestMatchBillsShort(t *testing.T) {
	bill := []domain.BillEntry{{TransactionID: "TXN9999", OutTradeNo: "HP9999", OrderAmount: 50}}

	rep := MatchBills(nil, bill)
	if len(rep.Short) != 1 || rep.Short[0].OrderNo != "HP9999" || rep.Short[0].Remote() != 50 || rep.Short[0].Reason != domain.ReasonRemoteOnly {
		t.Fatalf("expected 1 SHORT for HP9999, got %+v", rep.Short)
	}
	if len(rep.Long) != 0 || len(rep.Mismatch) != 0 {
		t.Fatalf("unexpected long/mismatch: %+v", rep)
	}
}

// TestMatchBillsMismatch 金额不一致 → MISMATCH。
func TestMatchBillsMismatch(t *testing.T) {
	local := []domain.LocalOrder{{OrderNo: "HP0001", ChannelTradeNo: "TXN1001", PaidAmount: 100}}
	bill := []domain.BillEntry{{TransactionID: "TXN1001", OutTradeNo: "HP0001", OrderAmount: 80}}

	rep := MatchBills(local, bill)
	if len(rep.Mismatch) != 1 || rep.Mismatch[0].Local() != 100 || rep.Mismatch[0].Remote() != 80 || rep.Mismatch[0].Reason != domain.ReasonMismatch {
		t.Fatalf("expected 1 MISMATCH 100 vs 80, got %+v", rep.Mismatch)
	}
	if len(rep.Long) != 0 || len(rep.Short) != 0 {
		t.Fatalf("unexpected long/short: %+v", rep)
	}
}

// TestMatchBillsFallbackOrderNo 本地渠道交易号缺失时按商户订单号匹配。
func TestMatchBillsFallbackOrderNo(t *testing.T) {
	local := []domain.LocalOrder{{OrderNo: "HP0001", ChannelTradeNo: "", PaidAmount: 100}}
	bill := []domain.BillEntry{{TransactionID: "TXN1001", OutTradeNo: "HP0001", OrderAmount: 100}}

	rep := MatchBills(local, bill)
	if len(rep.Long) != 0 || len(rep.Short) != 0 || len(rep.Mismatch) != 0 {
		t.Fatalf("expected matched via order_no fallback, got %+v", rep)
	}
}
