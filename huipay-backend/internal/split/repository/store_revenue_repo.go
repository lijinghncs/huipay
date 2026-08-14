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
	// ListUnsplitOrderNos 列出某时间范围内尚未分账的已支付订单号（用于分账单追溯覆盖订单）。
	ListUnsplitOrderNos(ctx context.Context, merchantID uint64, from, to time.Time) ([]string, error)
}

// StoreRevenueRepo 门店实收金额仓储（读 t_order 聚合）。
type StoreRevenueRepo struct{ db *gorm.DB }

// NewStoreRevenueRepo 构造 StoreRevenueRepo。
func NewStoreRevenueRepo(db *gorm.DB) *StoreRevenueRepo { return &StoreRevenueRepo{db: db} }

// splitExclusion 返回排除已分账订单的 SQL 片段与参数。
// 已分账订单判定：
//  1. 单笔分账：t_split_execution 中 order_no 相同且状态为 SUCCESS；
//  2. 时间段分账：被某张未驳回账单(order_nos 覆盖)记录，即 PENDING/APPROVED/EXECUTED 状态账单收录的订单。
//
// 返回表达式直接引用主表别名 o。
func splitExclusion() string {
	return `NOT EXISTS (
		SELECT 1 FROM t_split_execution se
		WHERE se.order_no = o.order_no AND se.status = 'SUCCESS'
	)
	AND NOT EXISTS (
		SELECT 1 FROM t_split_bill sb
		WHERE sb.merchant_id = o.merchant_id
		  AND sb.status IN ('PENDING','APPROVED','EXECUTED')
		  AND sb.order_nos IS NOT NULL
		  AND JSON_CONTAINS(sb.order_nos, JSON_QUOTE(o.order_no))
	)`
}

// SumPaidByStore 按门店聚合某时间范围内已支付订单实收金额。
// 仅统计尚未分账的订单；from/to 非零时仅统计 paid_at 落在 [from, to] 内的订单。
func (r *StoreRevenueRepo) SumPaidByStore(ctx context.Context, merchantID uint64, from, to time.Time) ([]StoreRevenue, error) {
	q := r.db.WithContext(ctx).Table("t_order AS o").
		Select("o.store_id, COALESCE(SUM(o.paid_amount), 0) AS paid").
		Where("o.merchant_id = ? AND o.status = 'PAID' AND o.deleted_at IS NULL AND o.store_id IS NOT NULL", merchantID).
		Where("o.paid_at IS NOT NULL").
		Where(splitExclusion())
	if !from.IsZero() {
		q = q.Where("o.paid_at >= ?", from)
	}
	if !to.IsZero() {
		q = q.Where("o.paid_at <= ?", to)
	}
	var rows []StoreRevenue
	if err := q.Group("o.store_id").Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// SumPaid 统计某时间范围内商户已支付订单实收总额（排除已分账订单）。
func (r *StoreRevenueRepo) SumPaid(ctx context.Context, merchantID uint64, from, to time.Time) (int64, error) {
	q := r.db.WithContext(ctx).Table("t_order AS o").
		Select("COALESCE(SUM(o.paid_amount), 0)").
		Where("o.merchant_id = ? AND o.status = 'PAID' AND o.deleted_at IS NULL", merchantID).
		Where("o.paid_at IS NOT NULL").
		Where(splitExclusion())
	if !from.IsZero() {
		q = q.Where("o.paid_at >= ?", from)
	}
	if !to.IsZero() {
		q = q.Where("o.paid_at <= ?", to)
	}
	var total int64
	if err := q.Scan(&total).Error; err != nil {
		return 0, err
	}
	return total, nil
}

// ListUnsplitOrderNos 列出某时间范围内尚未分账的已支付订单号。
func (r *StoreRevenueRepo) ListUnsplitOrderNos(ctx context.Context, merchantID uint64, from, to time.Time) ([]string, error) {
	q := r.db.WithContext(ctx).Table("t_order AS o").
		Select("o.order_no").
		Where("o.merchant_id = ? AND o.status = 'PAID' AND o.deleted_at IS NULL", merchantID).
		Where("o.paid_at IS NOT NULL").
		Where(splitExclusion())
	if !from.IsZero() {
		q = q.Where("o.paid_at >= ?", from)
	}
	if !to.IsZero() {
		q = q.Where("o.paid_at <= ?", to)
	}
	var orderNos []string
	if err := q.Scan(&orderNos).Error; err != nil {
		return nil, err
	}
	return orderNos, nil
}
