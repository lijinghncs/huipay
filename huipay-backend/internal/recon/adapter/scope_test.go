package adapter

import (
	"strings"
	"testing"
	"time"
)

func TestSameScopeFilter(t *testing.T) {
	from := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2024, 6, 3, 0, 0, 0, 0, time.UTC)

	where, args := SameScopeFilter(7, from, to)

	for _, want := range []string{
		"o.merchant_id = ?",
		"o.status = 'PAID'",
		"o.paid_at >= ?",
		"o.paid_at < ?",
		"o.deleted_at IS NULL",
		"s.status = 1",
		"se.status = 'SUCCESS'",
		"NOT EXISTS",
		"t_split_bill_biz_date",
	} {
		if !strings.Contains(where, want) {
			t.Errorf("SameScopeFilter missing %q", want)
		}
	}
	if len(args) != 3 {
		t.Fatalf("SameScopeFilter args = %d, want 3 (merchantID, from, to)", len(args))
	}
	if args[0] != uint64(7) {
		t.Errorf("args[0] = %v, want merchantID 7", args[0])
	}
}

func TestLayerQueries(t *testing.T) {
	if !strings.Contains(LayerAQuery, "SUM(o.paid_amount)") {
		t.Error("LayerAQuery should aggregate o.paid_amount")
	}
	if !strings.Contains(LayerBQuery, "GROUP BY DATE(o.paid_at), o.store_id") {
		t.Error("LayerBQuery should group by biz_date, store_id")
	}
	for _, q := range []string{LayerAQuery, LayerBQuery} {
		// 分账口径：排除已分账订单（LEFT JOIN + IS NULL 等价形式）
		for _, want := range []string{"se.order_no IS NULL", "bd.bill_id IS NULL", "o.status = 'PAID'", "o.deleted_at IS NULL"} {
			if !strings.Contains(q, want) {
				t.Errorf("layer query missing split-scope predicate %q", want)
			}
		}
	}
}

func TestStatsQueries(t *testing.T) {
	for _, q := range []string{StatsSumQuery, StatsRowsQuery} {
		for _, want := range []string{"t_store_daily_stats", "merchant_id = ?", "biz_date >= ?", "biz_date < ?"} {
			if !strings.Contains(q, want) {
				t.Errorf("stats query missing %q", want)
			}
		}
	}
	if !strings.Contains(StatsRowsQuery, "store_id") {
		t.Error("StatsRowsQuery should select store_id")
	}
	if !strings.Contains(StatsSumQuery, "stats_total") {
		t.Error("StatsSumQuery should alias result as stats_total")
	}
}
