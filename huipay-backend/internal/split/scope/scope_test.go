package scope

import (
	"strings"
	"testing"
	"time"
)

func TestSameScopeFilter(t *testing.T) {
	from := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)

	where, args := SameScopeFilter(100, from, to)

	if where == "" {
		t.Fatal("SameScopeFilter() returned empty where")
	}
	if len(args) != 3 {
		t.Fatalf("args length = %d, want 3", len(args))
	}
	merchantID, ok := args[0].(uint64)
	if !ok || merchantID != 100 {
		t.Errorf("args[0] = %v, want 100", args[0])
	}
	checks := []string{
		"merchant_id",
		"PAID",
		"deleted_at IS NULL",
		"paid_at",
		"store_id IS NOT NULL",
		"t_store",
		"t_split_execution",
		"t_split_bill",
	}
	for _, c := range checks {
		if !strings.Contains(where, c) {
			t.Errorf("SameScopeFilter where missing: %s", c)
		}
	}
}

func TestSameScopeFilter_TimeRange(t *testing.T) {
	from := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2024, 7, 1, 0, 0, 0, 0, time.UTC)

	_, args := SameScopeFilter(1, from, to)
	if len(args) < 3 {
		t.Fatal("args too short")
	}
	if args[1] != from {
		t.Errorf("args[1] = %v, want %v", args[1], from)
	}
	if args[2] != to {
		t.Errorf("args[2] = %v, want %v", args[2], to)
	}
}

func TestLayerAQuery(t *testing.T) {
	if LayerAQuery == "" {
		t.Fatal("LayerAQuery is empty")
	}
	checks := []string{"SUM", "t_store", "t_split_execution", "t_split_bill", "merchant_id", "PAID", "paid_at"}
	for _, c := range checks {
		if !strings.Contains(LayerAQuery, c) {
			t.Errorf("LayerAQuery missing: %s", c)
		}
	}
}

func TestLayerBQuery(t *testing.T) {
	if LayerBQuery == "" {
		t.Fatal("LayerBQuery is empty")
	}
	checks := []string{"biz_date", "store_id", "SUM", "GROUP BY"}
	for _, c := range checks {
		if !strings.Contains(LayerBQuery, c) {
			t.Errorf("LayerBQuery missing: %s", c)
		}
	}
}

func TestHasMissingQuery(t *testing.T) {
	if HasMissingQuery == "" {
		t.Fatal("HasMissingQuery is empty")
	}
	checks := []string{"DATE_ADD", "t_store_daily_stats", "merchant_id", "biz_date"}
	for _, c := range checks {
		if !strings.Contains(HasMissingQuery, c) {
			t.Errorf("HasMissingQuery missing: %s", c)
		}
	}
}

func TestStatsQueries(t *testing.T) {
	checks := []string{"merchant_id", "biz_date"}
	for _, c := range checks {
		if !strings.Contains(StatsSumQuery, c) {
			t.Errorf("StatsSumQuery missing: %s", c)
		}
		if !strings.Contains(StatsRowsQuery, c) {
			t.Errorf("StatsRowsQuery missing: %s", c)
		}
	}
}