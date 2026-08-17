package adapter

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// statsBizDateKey 日报侧聚合 key，与订单侧 "biz_date|store_id" 格式一致。
func statsBizDateKey(bizDate string, storeID uint64) string {
	return fmt.Sprintf("%s|%d", bizDate, storeID)
}

// StatsBackfiller 日报补跑器（stats 服务隐式满足；消费侧接口，避免 recon 反向依赖 stats 域）。
type StatsBackfiller interface {
	HasMissing(ctx context.Context, merchantID uint64, start, end time.Time) (bool, error)
	Backfill(ctx context.Context, start, end time.Time) (int64, error)
}

// StatsFetcher 日报侧取数适配器：Sum/Rows 为 t_store_daily_stats 只读投影，
// HasMissing/Backfill 委托注入的补跑器。
type StatsFetcher struct {
	db       *gorm.DB
	backfill StatsBackfiller
}

func NewStatsFetcher(db *gorm.DB, backfill StatsBackfiller) *StatsFetcher {
	return &StatsFetcher{db: db, backfill: backfill}
}

func (f *StatsFetcher) HasMissing(ctx context.Context, merchantID uint64, start, end time.Time) (bool, error) {
	return f.backfill.HasMissing(ctx, merchantID, start, end)
}

func (f *StatsFetcher) Backfill(ctx context.Context, start, end time.Time) (int64, error) {
	return f.backfill.Backfill(ctx, start, end)
}

// Sum 日报侧商户级总额。
func (f *StatsFetcher) Sum(ctx context.Context, merchantID uint64, start, end time.Time) (int64, error) {
	var total int64
	if err := f.db.WithContext(ctx).Raw(StatsSumQuery, merchantID, start, end).Scan(&total).Error; err != nil {
		return 0, err
	}
	return total, nil
}

// Rows 日报侧门店 × 日期聚合，key 为 "biz_date|store_id"（与订单侧 key 格式一致）。
func (f *StatsFetcher) Rows(ctx context.Context, merchantID uint64, start, end time.Time) (map[string]int64, error) {
	var rows []struct {
		BizDate string `gorm:"column:biz_date"`
		StoreID uint64 `gorm:"column:store_id"`
		Total   int64  `gorm:"column:total"`
	}
	if err := f.db.WithContext(ctx).Raw(StatsRowsQuery, merchantID, start, end).Scan(&rows).Error; err != nil {
		return nil, err
	}
	m := make(map[string]int64, len(rows))
	for _, r := range rows {
		m[statsBizDateKey(r.BizDate, r.StoreID)] = r.Total
	}
	return m, nil
}
