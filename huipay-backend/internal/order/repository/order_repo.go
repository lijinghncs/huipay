// 包 repository 提供订单数据访问。
package repository

import (
	"context"
	"errors"

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

// ListByMerchant 按商户号分页查询订单（created_at DESC）。
// status 为空表示不过滤状态。返回订单列表与总数。
func (r *OrderRepo) ListByMerchant(ctx context.Context, merchantID uint64, status string, page, size int) ([]model.OrderModel, int64, error) {
	if page < 1 {
		page = 1
	}
	if size <= 0 || size > 100 {
		size = 20
	}

	q := r.db.WithContext(ctx).Model(&model.OrderModel{})
	if status != "" {
		q = q.Where("status = ?", status)
	}

	var total int64
	if err := q.Where("merchant_id = ?", merchantID).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var rows []model.OrderModel
	if err := q.Where("merchant_id = ?", merchantID).
		Order("created_at DESC").
		Offset((page - 1) * size).
		Limit(size).
		Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}