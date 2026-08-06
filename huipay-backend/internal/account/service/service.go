// 包 service 提供账户与钱包业务编排。
package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"go.uber.org/zap"

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