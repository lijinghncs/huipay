// 包 repository 提供主体(entity)数据访问，面向商户进件与列表查询。
package repository

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
)

// EntityModel 主体表 GORM 模型（t_entity）。
type EntityModel struct {
	ID         uint64     `gorm:"column:id;primaryKey;autoIncrement"`
	EntityCode string     `gorm:"column:entity_code;size:32;uniqueIndex:uk_entity_code;not null"` // 商户号
	EntityType string     `gorm:"column:entity_type;size:32;not null"`                            // MERCHANT/STORE/...
	ParentID   *uint64    `gorm:"column:parent_id"`
	Name       string     `gorm:"column:name;size:128;not null"`
	KYCStatus  int        `gorm:"column:kyc_status;not null;default:0"`
	KYCData    string     `gorm:"column:kyc_data;type:json"`
	WechatConfig string  `gorm:"column:wechat_config;type:json"` // 商户微信支付配置（敏感字段已 AES 加密）
	SplitMode  string     `gorm:"column:split_mode;size:16;not null;default:AUTO"` // 分账模式：AUTO/LOCAL_ONLY/CHANNEL_REQUIRED
	LoginPhone string     `gorm:"column:login_phone;size:32;uniqueIndex:uk_login_phone"`      // 登录手机号
	LoginPasswordHash string `gorm:"column:login_password_hash;size:128"`                     // 登录密码哈希（bcrypt）
	Status     int        `gorm:"column:status;not null;default:1"`
	CreatedAt  time.Time  `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt  time.Time  `gorm:"column:updated_at;autoUpdateTime"`
	DeletedAt  *time.Time `gorm:"column:deleted_at"`
}

// TableName 表名。
func (EntityModel) TableName() string { return "t_entity" }

// ListFilter 列表筛选条件。
type ListFilter struct {
	Keyword string // 名称/商户号模糊
	Status  *int   // 状态（nil 表示不过滤）
	Offset  int
	Limit   int
}

// EntityRepo 主体仓储。
type EntityRepo struct{ db *gorm.DB }

// NewEntityRepo 构造 EntityRepo。
func NewEntityRepo(db *gorm.DB) *EntityRepo { return &EntityRepo{db: db} }

// DB 暴露主库用于事务。
func (r *EntityRepo) DB() *gorm.DB { return r.db }

// Create 创建主体。
func (r *EntityRepo) Create(ctx context.Context, m *EntityModel) error {
	return r.db.WithContext(ctx).Create(m).Error
}

// FindByCode 按商户号查询主体。
func (r *EntityRepo) FindByCode(ctx context.Context, code string) (*EntityModel, error) {
	var m EntityModel
	if err := r.db.WithContext(ctx).Where("entity_code = ?", code).First(&m).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &m, nil
}

// GetByID 按 ID 查询主体。
func (r *EntityRepo) GetByID(ctx context.Context, id uint64) (*EntityModel, error) {
	var m EntityModel
	if err := r.db.WithContext(ctx).First(&m, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &m, nil
}

// GetByLoginPhone 按登录手机号查询主体。
func (r *EntityRepo) GetByLoginPhone(ctx context.Context, phone string) (*EntityModel, error) {
	var m EntityModel
	if err := r.db.WithContext(ctx).Where("login_phone = ? AND deleted_at IS NULL", phone).First(&m).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &m, nil
}

// UpdateLoginPassword 更新登录密码哈希。
func (r *EntityRepo) UpdateLoginPassword(ctx context.Context, id uint64, phone, passwordHash string) error {
	return r.db.WithContext(ctx).Model(&EntityModel{}).
		Where("id = ?", id).
		Updates(map[string]any{"login_phone": phone, "login_password_hash": passwordHash}).Error
}

// UpdateProfile 更新主体基础资料（名称 + 商户身份认证资料 JSON）。
func (r *EntityRepo) UpdateProfile(ctx context.Context, id uint64, name, kycData string) error {
	return r.db.WithContext(ctx).Model(&EntityModel{}).
		Where("id = ?", id).
		Updates(map[string]any{"name": name, "kyc_data": kycData}).Error
}

// UpdateStatus 更新主体状态（启用/停用）。
func (r *EntityRepo) UpdateStatus(ctx context.Context, id uint64, status int) error {
	return r.db.WithContext(ctx).Model(&EntityModel{}).
		Where("id = ?", id).
		Update("status", status).Error
}

// UpdateWechatConfig 更新主体微信支付配置 JSON。
func (r *EntityRepo) UpdateWechatConfig(ctx context.Context, id uint64, cfgJSON string) error {
	if cfgJSON == "" {
		cfgJSON = "null"
	}
	return r.db.WithContext(ctx).Model(&EntityModel{}).
		Where("id = ?", id).
		Update("wechat_config", cfgJSON).Error
}

// List 分页查询商户主体（entity_type = MERCHANT）。
func (r *EntityRepo) List(ctx context.Context, f ListFilter) ([]EntityModel, int64, error) {
	q := r.db.WithContext(ctx).Model(&EntityModel{}).Where("entity_type = ?", "MERCHANT")
	if f.Keyword != "" {
		like := "%" + f.Keyword + "%"
		q = q.Where("(name LIKE ? OR entity_code LIKE ?)", like, like)
	}
	if f.Status != nil {
		q = q.Where("status = ?", *f.Status)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var list []EntityModel
	if err := q.Order("id DESC").Offset(f.Offset).Limit(f.Limit).Find(&list).Error; err != nil {
		return nil, 0, err
	}
	return list, total, nil
}
