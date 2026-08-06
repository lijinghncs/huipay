// 包 idem 实现幂等键中心。
// 存储使用 MySQL 的 t_idempotency_key 表（UNIQUE KEY uk_scope_key 兜底）。
package idem

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/huipay/huipay-backend/infra/errs"
)

// Record 幂等记录。
type Record struct {
	Scope       string `gorm:"column:scope;size:64;not null"`
	Key         string `gorm:"column:idempotency_key;size:64;not null"`
	RequestHash string `gorm:"column:request_hash;size:64;not null"`
	ResponseRaw string `gorm:"column:response_snapshot;type:json"`
	Status      int    `gorm:"column:status"`
	ExpireAt    time.Time `gorm:"column:expire_at"`
	CreatedAt   time.Time `gorm:"column:created_at"`
}

// TableName 表名。
func (Record) TableName() string { return "t_idempotency_key" }

// Store 幂等存储接口。
type Store interface {
	Save(ctx context.Context, r *Record) error
	Get(ctx context.Context, scope, key string) (*Record, error)
}

// MySQLStore 基于 GORM 的实现。
type MySQLStore struct{ db *gorm.DB }

// NewMySQLStore 构造 MySQLStore。
func NewMySQLStore(db *gorm.DB) *MySQLStore { return &MySQLStore{db: db} }

// Save 保存幂等记录，重复 key 报 BizError(CodeIdempotentConflict)。
func (s *MySQLStore) Save(ctx context.Context, r *Record) error {
	if err := s.db.WithContext(ctx).Create(r).Error; err != nil {
		// 唯一索引冲突 → 幂等命中
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return errs.New(errs.CodeIdempotentConflict, "idempotent key exists", 200)
		}
		return err
	}
	return nil
}

// Get 查询幂等记录。
func (s *MySQLStore) Get(ctx context.Context, scope, key string) (*Record, error) {
	var r Record
	if err := s.db.WithContext(ctx).
		Where("Record.scope = ? AND Record.idempotency_key = ?", scope, key).
		First(&r).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &r, nil
}