// 包 repository 提供门店(t_store)数据访问。
package repository

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
)

// StoreModel 门店表 GORM 模型（t_store）。
type StoreModel struct {
	ID           uint64    `gorm:"column:id;primaryKey;autoIncrement"`
	StoreCode    string    `gorm:"column:store_code;size:32;uniqueIndex:uk_store_code;not null"`
	MerchantID   uint64    `gorm:"column:merchant_id;not null"`
	Name         string    `gorm:"column:name;size:64;not null"`
	StoreType    string    `gorm:"column:store_type;size:16"`
	ContactPhone string    `gorm:"column:contact_phone;size:32"`
	Region       string    `gorm:"column:region;size:128"`
	Address      string    `gorm:"column:address;size:256"`
	Longitude    *float64  `gorm:"column:longitude"`
	Latitude     *float64  `gorm:"column:latitude"`
	Metadata     string    `gorm:"column:metadata;type:json"`
	Status       int       `gorm:"column:status;not null;default:1"`
	CreatedAt    time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt    time.Time `gorm:"column:updated_at;autoUpdateTime"`
	DeletedAt    *time.Time `gorm:"column:deleted_at"`
	// 非持久字段：关联码牌数/订单数（列表查询聚合填充，只读）
	CodeCount  int64 `gorm:"->"`
	OrderCount int64 `gorm:"->"`
}

// TableName 表名。
func (StoreModel) TableName() string { return "t_store" }

// ListFilter 列表筛选条件。
type ListFilter struct {
	Keyword string // 名称/编码模糊
	Status  *int   // 状态（nil 表示不过滤）
	Offset  int
	Limit   int
}

// StoreRepo 门店仓储。
type StoreRepo struct{ db *gorm.DB }

// NewStoreRepo 构造 StoreRepo。
func NewStoreRepo(db *gorm.DB) *StoreRepo { return &StoreRepo{db: db} }

// DB 暴露主库用于事务与关联查询。
func (r *StoreRepo) DB() *gorm.DB { return r.db }

// Create 创建门店。
func (r *StoreRepo) Create(ctx context.Context, m *StoreModel) error {
	return r.db.WithContext(ctx).Create(m).Error
}

// GetByStoreCode 按门店编码查询。
func (r *StoreRepo) GetByStoreCode(ctx context.Context, code string) (*StoreModel, error) {
	var m StoreModel
	if err := r.db.WithContext(ctx).Where("store_code = ?", code).First(&m).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &m, nil
}

// GetByIDAndMerchant 按 ID + 所属商户查询（用于权限校验）。
func (r *StoreRepo) GetByIDAndMerchant(ctx context.Context, id, merchantID uint64) (*StoreModel, error) {
	var m StoreModel
	if err := r.db.WithContext(ctx).Where("id = ? AND merchant_id = ?", id, merchantID).First(&m).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &m, nil
}

// ListByMerchant 分页查询商户门店。
func (r *StoreRepo) ListByMerchant(ctx context.Context, merchantID uint64, f ListFilter) ([]StoreModel, int64, error) {
	q := r.db.WithContext(ctx).Model(&StoreModel{}).Where("t_store.merchant_id = ? AND t_store.deleted_at IS NULL", merchantID)
	if f.Keyword != "" {
		like := "%" + f.Keyword + "%"
		q = q.Where("(t_store.name LIKE ? OR t_store.store_code LIKE ?)", like, like)
	}
	if f.Status != nil {
		q = q.Where("t_store.status = ?", *f.Status)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var list []StoreModel
	err := q.Select("t_store.*, COALESCE(pc.code_count, 0) AS code_count, COALESCE(od.order_count, 0) AS order_count").
		Joins("LEFT JOIN (SELECT store_id, COUNT(*) AS code_count FROM t_payment_code WHERE deleted_at IS NULL GROUP BY store_id) pc ON pc.store_id = t_store.id").
		Joins("LEFT JOIN (SELECT store_id, COUNT(*) AS order_count FROM t_order WHERE deleted_at IS NULL GROUP BY store_id) od ON od.store_id = t_store.id").
		Order("t_store.id DESC").
		Offset(f.Offset).Limit(f.Limit).
		Find(&list).Error
	if err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

// CountByMerchant 统计商户门店数量（不含已删除）。
func (r *StoreRepo) CountByMerchant(ctx context.Context, merchantID uint64) (int64, error) {
	var total int64
	err := r.db.WithContext(ctx).Model(&StoreModel{}).Where("merchant_id = ? AND deleted_at IS NULL", merchantID).Count(&total).Error
	return total, err
}

// CountByMerchantStatus 统计商户指定状态门店数量（不含已删除）。
func (r *StoreRepo) CountByMerchantStatus(ctx context.Context, merchantID uint64, status int) (int64, error) {
	var total int64
	err := r.db.WithContext(ctx).Model(&StoreModel{}).Where("merchant_id = ? AND status = ? AND deleted_at IS NULL", merchantID, status).Count(&total).Error
	return total, err
}

// CountByMerchantCreatedAfter 统计商户某时间之后创建的门店数量（不含已删除）。
func (r *StoreRepo) CountByMerchantCreatedAfter(ctx context.Context, merchantID uint64, after string) (int64, error) {
	var total int64
	err := r.db.WithContext(ctx).Model(&StoreModel{}).Where("merchant_id = ? AND created_at >= ? AND deleted_at IS NULL", merchantID, after).Count(&total).Error
	return total, err
}

// Update 更新门店非空字段。
func (r *StoreRepo) Update(ctx context.Context, id, merchantID uint64, fields map[string]any) error {
	return r.db.WithContext(ctx).Model(&StoreModel{}).
		Where("id = ? AND merchant_id = ?", id, merchantID).
		Updates(fields).Error
}

// UpdateStatus 更新门店状态。
func (r *StoreRepo) UpdateStatus(ctx context.Context, id, merchantID uint64, status int) error {
	return r.db.WithContext(ctx).Model(&StoreModel{}).
		Where("id = ? AND merchant_id = ?", id, merchantID).
		Update("status", status).Error
}

// SoftDelete 软删除门店。
func (r *StoreRepo) SoftDelete(ctx context.Context, id, merchantID uint64) error {
	now := time.Now()
	return r.db.WithContext(ctx).Model(&StoreModel{}).
		Where("id = ? AND merchant_id = ?", id, merchantID).
		Update("deleted_at", now).Error
}

// CountCodesByStore 统计关联门店的码牌数（含逻辑删除过滤）。
func (r *StoreRepo) CountCodesByStore(ctx context.Context, storeID uint64) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Raw(
		"SELECT COUNT(*) FROM t_payment_code WHERE store_id = ? AND deleted_at IS NULL", storeID,
	).Scan(&n).Error
	return n, err
}

// CountOrdersByStore 统计关联门店的订单数。
func (r *StoreRepo) CountOrdersByStore(ctx context.Context, storeID uint64) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Raw(
		"SELECT COUNT(*) FROM t_order WHERE store_id = ? AND deleted_at IS NULL", storeID,
	).Scan(&n).Error
	return n, err
}

// AuditLogItem 审计日志模型。
type AuditLogItem struct {
	StoreID    uint64 `gorm:"column:store_id;not null"`
	MerchantID uint64 `gorm:"column:merchant_id;not null"`
	Action     string `gorm:"column:action;size:32;not null"`
	Operator   string `gorm:"column:operator;size:64"`
	Detail     string `gorm:"column:detail;type:json"`
}

// TableName 表名。
func (AuditLogItem) TableName() string { return "t_store_audit_log" }

// AuditLog 写入门店审计日志。
func (r *StoreRepo) AuditLog(ctx context.Context, m *AuditLogItem) error {
	return r.db.WithContext(ctx).Create(m).Error
}