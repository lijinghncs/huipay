// 包 repository 提供收款码牌数据访问。
package repository

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
)

// PaymentCodeModel 收款码牌表 GORM 模型（t_payment_code）。
type PaymentCodeModel struct {
	ID         uint64     `gorm:"column:id;primaryKey;autoIncrement"`
	MerchantID uint64     `gorm:"column:merchant_id;not null"`
	CodeID     string     `gorm:"column:code_id;size:16;uniqueIndex:uk_code_id;not null"`
	Status     int        `gorm:"column:status;not null;default:1"`
	Remark     string     `gorm:"column:remark;size:64"`
	CreatedAt  time.Time  `gorm:"column:created_at;autoCreateTime"`
	DisabledAt *time.Time `gorm:"column:disabled_at"`
	DeletedAt  *time.Time `gorm:"column:deleted_at"`
}

// TableName 表名。
func (PaymentCodeModel) TableName() string { return "t_payment_code" }

// PaymentCodeFilter 列表筛选条件。
type PaymentCodeFilter struct {
	MerchantID uint64 // 所属商户（必填）
	Status     *int   // 状态（nil 表示不过滤）
	Offset     int
	Limit      int
}

// PaymentCodeRepo 收款码牌仓储。
type PaymentCodeRepo struct{ db *gorm.DB }

// NewPaymentCodeRepo 构造 PaymentCodeRepo。
func NewPaymentCodeRepo(db *gorm.DB) *PaymentCodeRepo { return &PaymentCodeRepo{db: db} }

// DB 暴露主库用于事务。
func (r *PaymentCodeRepo) DB() *gorm.DB { return r.db }

// Create 创建码牌。
func (r *PaymentCodeRepo) Create(ctx context.Context, m *PaymentCodeModel) error {
	return r.db.WithContext(ctx).Create(m).Error
}

// GetByCodeID 按对外短码查询码牌（含已停用）。
func (r *PaymentCodeRepo) GetByCodeID(ctx context.Context, codeID string) (*PaymentCodeModel, error) {
	var m PaymentCodeModel
	if err := r.db.WithContext(ctx).Where("code_id = ?", codeID).First(&m).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &m, nil
}

// GetByIDAndMerchant 按时牌 ID + 所属商户查询（用于权限校验）。
func (r *PaymentCodeRepo) GetByIDAndMerchant(ctx context.Context, id, merchantID uint64) (*PaymentCodeModel, error) {
	var m PaymentCodeModel
	if err := r.db.WithContext(ctx).Where("id = ? AND merchant_id = ?", id, merchantID).First(&m).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &m, nil
}

// ListByMerchant 分页查询商户码牌。
func (r *PaymentCodeRepo) ListByMerchant(ctx context.Context, f PaymentCodeFilter) ([]PaymentCodeModel, int64, error) {
	q := r.db.WithContext(ctx).Model(&PaymentCodeModel{}).Where("merchant_id = ?", f.MerchantID)
	if f.Status != nil {
		q = q.Where("status = ?", *f.Status)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var list []PaymentCodeModel
	if err := q.Order("id DESC").Offset(f.Offset).Limit(f.Limit).Find(&list).Error; err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

// Disable 停用码牌（仅启用状态更新，返回是否真的更新）。
func (r *PaymentCodeRepo) Disable(ctx context.Context, id uint64) (bool, error) {
	now := time.Now()
	res := r.db.WithContext(ctx).
		Model(&PaymentCodeModel{}).
		Where("id = ? AND status = 1", id).
		Updates(map[string]any{"status": 0, "disabled_at": now})
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
}