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
	ID         uint64    `gorm:"column:id;primaryKey;autoIncrement"`
	WalletNo   string    `gorm:"column:wallet_no;size:32;uniqueIndex:uk_wallet_no;not null"`
	EntityID   uint64    `gorm:"column:entity_id;not null"`
	EntityType string    `gorm:"column:entity_type;size:32;not null"`
	Currency   string    `gorm:"column:currency;size:3;not null;default:CNY"`
	Balance    int64     `gorm:"column:balance;not null;default:0"`
	Frozen     int64     `gorm:"column:frozen;not null;default:0"`
	PreFrozen  int64     `gorm:"column:pre_frozen;not null;default:0"`
	Version    int64     `gorm:"column:version;not null;default:0"`
	Status     int       `gorm:"column:status;not null;default:1"`
	CreatedAt  time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt  time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

// TableName 表名。
func (WalletModel) TableName() string { return "t_wallet" }

// JournalEntryModel 账本流水 GORM 模型。
type JournalEntryModel struct {
	ID             string    `gorm:"column:id;primaryKey;size:20"`
	WalletID       uint64    `gorm:"column:wallet_id;not null"`
	Direction      string    `gorm:"column:direction;type:ENUM('DEBIT','CREDIT');not null"`
	Amount         int64     `gorm:"column:amount;not null"`
	BalanceAfter   int64     `gorm:"column:balance_after;not null"`
	BizType        string    `gorm:"column:biz_type;size:32;not null"`
	BizID          string    `gorm:"column:biz_id;size:64;not null"`
	CounterpartyID *uint64   `gorm:"column:counterparty_id"`
	IdempotencyKey string    `gorm:"column:idempotency_key;size:64;not null"`
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

// GetByEntity 按主体查询钱包。
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

// DB 暴露主库用于事务。
func (r *JournalRepo) DB() *gorm.DB { return r.db }