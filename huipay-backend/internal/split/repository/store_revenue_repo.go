// 包 repository 提供门店实收金额查询。
package repository

import (
	"context"
	"time"

	"gorm.io/gorm"
)

// StoreRevenue 门店实收金额。
type StoreRevenue struct {
	StoreID uint64 `gorm:"column:store_id"`
	Paid    int64  `gorm:"column:paid"`
}

// StoreRevenueQuerier 查询商户门店实收金额（已支付订单，按时间范围）。
type StoreRevenueQuerier interface {
	// SumPaidByStore 按门店聚合某时间范围内已支付订单实收金额。
	SumPaidByStore(ctx context.Context, merchantID uint64, from, to time.Time) ([]StoreRevenue, error)
	// SumPaid 统计某时间范围内商户已支付订单实收总额。
	SumPaid(ctx context.Context, merchantID uint64, from, to time.Time) (int64, error)
}

// StoreRevenueRepo 门店实收金额仓储（读 t_order 聚合）。
type StoreRevenueRepo struct{ db *gorm.DB }

// NewStoreRevenueRepo 构造 StoreRevenueRepo。
func NewStoreRevenueRepo(db *gorm.DB) *StoreRevenueRepo { return &StoreRevenueRepo{db: db} }

// SumPaidByStore 按门店聚合某时间范围内已支付订单实收金额。
// from/to 非零时仅统计 paid_at 落在 [from, to] 内的订单（无支付时间的订单不计入）。
func (r *StoreRevenueRepo) SumPaidByStore(ctx context.Context, merchantID uint64, from, to time.Time) ([]StoreRevenue, error) {
	q := r.db.WithContext(ctx).Table("t_order").
		Select("store_id, COALESCE(SUM(paid_amount), 0) AS paid").
		Where("merchant_id = ? AND status = 'PAID' AND deleted_at IS NULL AND store_id IS NOT NULL", merchantID).
		Where("paid_at IS NOT NULL")
	if !from.IsZero() {
		q = q.Where("paid_at >= ?", from)
	}
	if !to.IsZero() {
		q = q.Where("paid_at <= ?", to)
	}
	var rows []StoreRevenue
	if err := q.Group("store_id").Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// SumPaid 统计某时间范围内商户已支付订单实收总额。
func (r *StoreRevenueRepo) SumPaid(ctx context.Context, merchantID uint64, from, to time.Time) (int64, error) {
	q := r.db.WithContext(ctx).Table("t_order").
		Select("COALESCE(SUM(paid_amount), 0)").
		Where("merchant_id = ? AND status = 'PAID' AND deleted_at IS NULL", merchantID).
		Where("paid_at IS NOT NULL")
	if !from.IsZero() {
		q = q.Where("paid_at >= ?", from)
	}
	if !to.IsZero() {
		q = q.Where("paid_at <= ?", to)
	}
	var total int64
	if err := q.Scan(&total).Error; err != nil {
		return 0, err
	}
	return total, nil
}
