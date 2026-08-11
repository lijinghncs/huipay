// 包 service 提供账户与钱包业务编排。
package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/huipay/huipay-backend/internal/account/ledger"
	"github.com/huipay/huipay-backend/internal/account/repository"
	"github.com/huipay/huipay-backend/internal/domain/vo"
)

// CreditRequest 入账请求（如：分账到账、提现失败退款入账）。
type CreditRequest struct {
	EntityID       uint64
	EntityType     vo.EntityType
	Amount         int64
	BizType        string
	BizID          string
	IdempotencyKey string
	TraceID        string
}

// Service 账户服务。
type Service struct {
	ledger     *ledger.Service
	walletRepo *repository.WalletRepo
	logger     *zap.Logger
	journalRepo *repository.JournalRepo
}

// NewService 构造 Service。
func NewService(l *ledger.Service, wr *repository.WalletRepo, jr *repository.JournalRepo, logger *zap.Logger) *Service {
	return &Service{ledger: l, walletRepo: wr, journalRepo: jr, logger: logger}
}

// GetWallet 查询钱包。
func (s *Service) GetWallet(ctx context.Context, entityID uint64) (*repository.WalletModel, error) {
	w, err := s.walletRepo.GetByEntity(ctx, entityID)
	if err != nil {
		return nil, err
	}
	if w == nil {
		return nil, errors.New("wallet not found")
	}
	return w, nil
}

// EnsureWalletTx 在指定事务内确保主体钱包存在（创建或返回已存在），用于与主体创建同事务提交。
func (s *Service) EnsureWalletTx(ctx context.Context, tx *gorm.DB, entityID uint64, entityType vo.EntityType) (*repository.WalletModel, error) {
	var w repository.WalletModel
	err := tx.WithContext(ctx).Where("entity_id = ?", entityID).First(&w).Error
	if err == nil {
		return &w, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	m := &repository.WalletModel{
		WalletNo:   "W" + uuid.NewString()[:16],
		EntityID:   entityID,
		EntityType: string(entityType),
		Currency:   "CNY",
		Status:     1,
	}
	if err := tx.WithContext(ctx).Create(m).Error; err != nil {
		// 并发创建时唯一索引冲突 → 重新查
		if err2 := tx.WithContext(ctx).Where("entity_id = ?", entityID).First(&w).Error; err2 == nil {
			return &w, nil
		}
		return nil, err
	}
	return m, nil
}

// ListEntries 列账本流水。
func (s *Service) ListEntries(ctx context.Context, entityID uint64, limit int) ([]repository.JournalEntryModel, error) {
	w, err := s.walletRepo.GetByEntity(ctx, entityID)
	if err != nil {
		return nil, err
	}
	if w == nil {
		return nil, errors.New("wallet not found")
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	return s.journalRepo.ListByWallet(ctx, w.ID, limit)
}

// Entry 账本流水 DTO（对外返回 snake_case 字段）。
type Entry struct {
	ID             string    `json:"id"`
	WalletID       uint64    `json:"wallet_id"`
	Direction      string    `json:"direction"`
	Amount         int64     `json:"amount"`
	BalanceAfter   int64     `json:"balance_after"`
	BizType        string    `json:"biz_type"`
	BizID          string    `json:"biz_id"`
	CounterpartyID *uint64   `json:"counterparty_id,omitempty"`
	IdempotencyKey string    `json:"idempotency_key"`
	TraceID        string    `json:"trace_id,omitempty"`
	Remark         string    `json:"remark,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

// EntryList 流水分页列表。
type EntryList struct {
	Items []Entry `json:"items"`
	Total int64   `json:"total"`
	Page  int     `json:"page"`
	Size  int     `json:"size"`
}

// EntryQuery 流水查询条件。
type EntryQuery struct {
	BizType string     // 空 = 不过滤
	BizID   string     // 空 = 不过滤
	Start   *time.Time // 空 = 不过滤
	End     *time.Time // 空 = 不过滤
	Page    int
	Size    int
}

// ListEntriesFiltered 按条件分页列流水（支持类型 / 订单号 / 时间区间过滤）。
func (s *Service) ListEntriesFiltered(ctx context.Context, entityID uint64, q EntryQuery) (*EntryList, error) {
	w, err := s.walletRepo.GetByEntity(ctx, entityID)
	if err != nil {
		return nil, err
	}
	if w == nil {
		return nil, errors.New("wallet not found")
	}
	if q.Page <= 0 {
		q.Page = 1
	}
	if q.Size <= 0 || q.Size > 200 {
		q.Size = 50
	}
	f := repository.EntryFilter{
		WalletID: w.ID,
		BizType:  q.BizType,
		BizID:    q.BizID,
		Start:    q.Start,
		End:      q.End,
		Offset:   (q.Page - 1) * q.Size,
		Limit:    q.Size,
	}
	models, err := s.journalRepo.ListByFilter(ctx, f)
	if err != nil {
		return nil, err
	}
	items := make([]Entry, 0, len(models))
	for i := range models {
		m := &models[i]
		items = append(items, Entry{
			ID:             m.ID,
			WalletID:       m.WalletID,
			Direction:      m.Direction,
			Amount:         m.Amount,
			BalanceAfter:   m.BalanceAfter,
			BizType:        m.BizType,
			BizID:          m.BizID,
			CounterpartyID: m.CounterpartyID,
			IdempotencyKey: m.IdempotencyKey,
			TraceID:        m.TraceID,
			Remark:         m.Remark,
			CreatedAt:      m.CreatedAt,
		})
	}
	total, err := s.journalRepo.CountByFilter(ctx, f)
	if err != nil {
		return nil, err
	}
	return &EntryList{Items: items, Total: total, Page: q.Page, Size: q.Size}, nil
}

// EnsureWallet 确保主体钱包存在（创建或返回已存在）。
func (s *Service) EnsureWallet(ctx context.Context, entityID uint64, entityType vo.EntityType) (*repository.WalletModel, error) {
	w, err := s.walletRepo.GetByEntity(ctx, entityID)
	if err != nil {
		return nil, err
	}
	if w != nil {
		return w, nil
	}
	m := &repository.WalletModel{
		WalletNo:   "W" + uuid.NewString()[:16],
		EntityID:   entityID,
		EntityType: string(entityType),
		Currency:   "CNY",
		Status:     1,
	}
	if err := s.walletRepo.DB().WithContext(ctx).Create(m).Error; err != nil {
		// 并发创建时唯一索引冲突 → 重新查
		w2, _ := s.walletRepo.GetByEntity(ctx, entityID)
		if w2 != nil {
			return w2, nil
		}
		return nil, fmt.Errorf("create wallet: %w", err)
	}
	return m, nil
}