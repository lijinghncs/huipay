// 包 repository 提供钱包与账本数据访问。
package repository

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
)

// WalletModel 钱包表 GORM 模型。
type WalletModel struct {
	ID         uint64    `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	WalletNo   string    `gorm:"column:wallet_no;size:32;uniqueIndex:uk_wallet_no;not null" json:"wallet_no"`
	EntityID   uint64    `gorm:"column:entity_id;not null" json:"entity_id"`
	EntityType string    `gorm:"column:entity_type;size:32;not null" json:"entity_type"`
	Currency   string    `gorm:"column:currency;size:3;not null;default:CNY" json:"currency"`
	Balance    int64     `gorm:"column:balance;not null;default:0" json:"balance"`
	Frozen     int64     `gorm:"column:frozen;not null;default:0" json:"frozen"`
	PreFrozen  int64     `gorm:"column:pre_frozen;not null;default:0" json:"pre_frozen"`
	Version    int64     `gorm:"column:version;not null;default:0" json:"version"`
	Status     int       `gorm:"column:status;not null;default:1" json:"status"`
	CreatedAt  time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt  time.Time `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
}

// TableName 表名。
func (WalletModel) TableName() string { return "t_wallet" }

// JournalEntryModel 账本流水 GORM 模型。
type JournalEntryModel struct {
	ID             string    `gorm:"column:id;primaryKey;size:20"`
	WalletID       uint64    `gorm:"column:wallet_id;not null;uniqueIndex:uk_idem"`
	Direction      string    `gorm:"column:direction;size:8;not null"`
	Amount         int64     `gorm:"column:amount;not null"`
	BalanceAfter   int64     `gorm:"column:balance_after;not null"`
	BizType        string    `gorm:"column:biz_type;size:32;not null"`
	BizID          string    `gorm:"column:biz_id;size:64;not null"`
	CounterpartyID *uint64   `gorm:"column:counterparty_id"`
	IdempotencyKey string    `gorm:"column:idempotency_key;size:64;not null;uniqueIndex:uk_idem"`
	TraceID        string    `gorm:"column:trace_id;size:64"`
	Remark         string    `gorm:"column:remark;size:255"`
	CreatedAt      time.Time `gorm:"column:created_at;autoCreateTime"`
}

// TableName 表名。
func (JournalEntryModel) TableName() string { return "t_journal_entry" }

// WalletRepo 钱包仓储。
type WalletRepo struct{ db *gorm.DB }

// NewWalletRepo 构造 WalletRepo。
func NewWalletRepo(db *gorm.DB) *WalletRepo { return &WalletRepo{db: db} }

// GetByEntity 按主体查询钱包（默认 MERCHANT 等单主体场景；多主体类型请用 GetByEntityType）。
func (r *WalletRepo) GetByEntity(ctx context.Context, entityID uint64) (*WalletModel, error) {
	var m WalletModel
	if err := r.db.WithContext(ctx).Where("entity_id = ?", entityID).First(&m).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &m, nil
}

// GetByEntityType 按主体 ID + 类型查询钱包（门店/商户/平台等类型隔离，避免 id 自增空间冲突）。
func (r *WalletRepo) GetByEntityType(ctx context.Context, entityID uint64, entityType string) (*WalletModel, error) {
	var m WalletModel
	if err := r.db.WithContext(ctx).Where("entity_id = ? AND entity_type = ?", entityID, entityType).First(&m).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &m, nil
}

// GetByID 按 ID 查询钱包（用于带乐观锁更新）。
func (r *WalletRepo) GetByID(ctx context.Context, id uint64) (*WalletModel, error) {
	var m WalletModel
	if err := r.db.WithContext(ctx).First(&m, id).Error; err != nil {
		return nil, err
	}
	return &m, nil
}

// UpdateBalance 原子更新余额（带乐观锁）。
func (r *WalletRepo) UpdateBalance(ctx context.Context, id uint64, version int64, newBalance int64) error {
	res := r.db.WithContext(ctx).
		Model(&WalletModel{}).
		Where("id = ? AND version = ?", id, version).
		Updates(map[string]any{"balance": newBalance, "version": version + 1})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errors.New("optimistic lock conflict")
	}
	return nil
}

// DB 暴露读主库。
func (r *WalletRepo) DB() *gorm.DB { return r.db }

// GetByIDTx 在指定事务内按 ID 查询钱包。
func (r *WalletRepo) GetByIDTx(ctx context.Context, tx *gorm.DB, id uint64) (*WalletModel, error) {
	var m WalletModel
	if err := tx.WithContext(ctx).First(&m, id).Error; err != nil {
		return nil, err
	}
	return &m, nil
}

// GetByEntityTx 在指定事务内按主体查询钱包。
func (r *WalletRepo) GetByEntityTx(ctx context.Context, tx *gorm.DB, entityID uint64) (*WalletModel, error) {
	var m WalletModel
	if err := tx.WithContext(ctx).Where("entity_id = ?", entityID).First(&m).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &m, nil
}

// GetByEntityTypeTx 在指定事务内按主体 ID + 类型查询钱包。
func (r *WalletRepo) GetByEntityTypeTx(ctx context.Context, tx *gorm.DB, entityID uint64, entityType string) (*WalletModel, error) {
	var m WalletModel
	if err := tx.WithContext(ctx).Where("entity_id = ? AND entity_type = ?", entityID, entityType).First(&m).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &m, nil
}

// UpdateBalanceTx 在指定事务内原子更新余额（带乐观锁）。
func (r *WalletRepo) UpdateBalanceTx(ctx context.Context, tx *gorm.DB, id uint64, version int64, newBalance int64) error {
	res := tx.WithContext(ctx).
		Model(&WalletModel{}).
		Where("id = ? AND version = ?", id, version).
		Updates(map[string]any{"balance": newBalance, "version": version + 1})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errors.New("optimistic lock conflict")
	}
	return nil
}

// JournalRepo 账本流水仓储。
type JournalRepo struct{ db *gorm.DB }

// NewJournalRepo 构造 JournalRepo。
func NewJournalRepo(db *gorm.DB) *JournalRepo { return &JournalRepo{db: db} }

// Append 追加流水（唯一键 wallet_id+idempotency_key 兜底幂等）。
func (r *JournalRepo) Append(ctx context.Context, m *JournalEntryModel) error {
	return r.db.WithContext(ctx).Create(m).Error
}

// ListByWallet 按钱包列流水。
func (r *JournalRepo) ListByWallet(ctx context.Context, walletID uint64, limit int) ([]JournalEntryModel, error) {
	var list []JournalEntryModel
	if err := r.db.WithContext(ctx).
		Where("wallet_id = ?", walletID).
		Order("created_at DESC").
		Limit(limit).
		Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// EntryFilter 流水过滤条件。
type EntryFilter struct {
	WalletID uint64
	BizType  string // 空 = 不过滤
	BizID    string // 空 = 不过滤
	Start    *time.Time // 空 = 不过滤
	End      *time.Time // 空 = 不过滤
	Offset   int
	Limit    int
}

// ListByFilter 按钱包 + 可选条件（业务类型 / 业务 ID / 时间区间）分页列流水。
// 复用索引 idx_biz(biz_type, biz_id) 与 idx_wallet_created。
func (r *JournalRepo) ListByFilter(ctx context.Context, f EntryFilter) ([]JournalEntryModel, error) {
	q := r.db.WithContext(ctx).Model(&JournalEntryModel{}).Where("wallet_id = ?", f.WalletID)
	if f.BizType != "" {
		q = q.Where("biz_type = ?", f.BizType)
	}
	if f.BizID != "" {
		q = q.Where("biz_id = ?", f.BizID)
	}
	if f.Start != nil {
		q = q.Where("created_at >= ?", *f.Start)
	}
	if f.End != nil {
		q = q.Where("created_at < ?", *f.End)
	}
	if f.Limit <= 0 || f.Limit > 200 {
		f.Limit = 50
	}
	var list []JournalEntryModel
	if err := q.Order("created_at DESC").Offset(f.Offset).Limit(f.Limit).Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// CountByFilter 统计满足过滤条件的流水数量（用于分页 total）。
func (r *JournalRepo) CountByFilter(ctx context.Context, f EntryFilter) (int64, error) {
	q := r.db.WithContext(ctx).Model(&JournalEntryModel{}).Where("wallet_id = ?", f.WalletID)
	if f.BizType != "" {
		q = q.Where("biz_type = ?", f.BizType)
	}
	if f.BizID != "" {
		q = q.Where("biz_id = ?", f.BizID)
	}
	if f.Start != nil {
		q = q.Where("created_at >= ?", *f.Start)
	}
	if f.End != nil {
		q = q.Where("created_at < ?", *f.End)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return 0, err
	}
	return total, nil
}

// DB 暴露主库用于事务。
func (r *JournalRepo) DB() *gorm.DB { return r.db }