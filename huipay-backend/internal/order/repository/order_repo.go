// 包 repository 提供订单数据访问。
package repository

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/huipay/huipay-backend/internal/order/model"
)

// OrderRepo 订单仓储。
type OrderRepo struct{ db *gorm.DB }

// NewOrderRepo 构造 OrderRepo。
func NewOrderRepo(db *gorm.DB) *OrderRepo { return &OrderRepo{db: db} }

// Create 创建订单（依赖 t_order 表的 UNIQUE KEY uk_merchant_order 兜底幂等）。
func (r *OrderRepo) Create(ctx context.Context, m *model.OrderModel) error {
	return r.db.WithContext(ctx).Create(m).Error
}

// GetByOrderNo 按平台订单号查询。
func (r *OrderRepo) GetByOrderNo(ctx context.Context, no string) (*model.OrderModel, error) {
	var m model.OrderModel
	if err := r.db.WithContext(ctx).Where("order_no = ?", no).First(&m).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &m, nil
}

// GetByMerchantOrder 按商户号 + 商户单号查询（幂等键）。
func (r *OrderRepo) GetByMerchantOrder(ctx context.Context, merchantID uint64, mOrderNo string) (*model.OrderModel, error) {
	var m model.OrderModel
	if err := r.db.WithContext(ctx).
		Where("merchant_id = ? AND merchant_order_no = ?", merchantID, mOrderNo).
		First(&m).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &m, nil
}

// UpdateStatus 更新订单状态（乐观锁）。
func (r *OrderRepo) UpdateStatus(ctx context.Context, orderNo, status, splitStatus string) error {
	return r.db.WithContext(ctx).
		Model(&model.OrderModel{}).
		Where("order_no = ?", orderNo).
		Updates(map[string]any{"status": status, "split_status": splitStatus}).Error
}

// OrderListFilter 订单列表过滤条件。
type OrderListFilter struct {
	MerchantID uint64
	Status     string     // 空 = 全部状态
	CodeID     string     // 来源收款码短码（可选）
	Channel    string     // 支付通道（可选）
	Start      *time.Time // 创建时间起（可选）
	End        *time.Time // 创建时间止（可选）
	Page       int
	Size       int
}

// ListByMerchant 按商户号分页查询订单（created_at DESC），支持状态/码牌/通道/时间过滤。
func (r *OrderRepo) ListByMerchant(ctx context.Context, f OrderListFilter) ([]model.OrderModel, int64, error) {
	if f.Page < 1 {
		f.Page = 1
	}
	if f.Size <= 0 || f.Size > 100 {
		f.Size = 20
	}

	q := r.db.WithContext(ctx).Model(&model.OrderModel{})
	q = q.Where("merchant_id = ?", f.MerchantID)
	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}
	if f.CodeID != "" {
		q = q.Where("code_id = ?", f.CodeID)
	}
	if f.Channel != "" {
		q = q.Where("channel = ?", f.Channel)
	}
	if f.Start != nil {
		q = q.Where("created_at >= ?", *f.Start)
	}
	if f.End != nil {
		q = q.Where("created_at <= ?", *f.End)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var rows []model.OrderModel
	if err := q.Order("created_at DESC").Offset((f.Page - 1) * f.Size).Limit(f.Size).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}
