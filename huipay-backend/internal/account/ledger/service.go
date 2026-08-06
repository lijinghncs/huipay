// 包 ledger 实现复式记账核心能力。
// 原则：每笔资金变动至少生成 2 条流水（借贷平衡），保证可追溯、可校验。
package ledger

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/huipay/huipay-backend/infra/prom"
	"github.com/huipay/huipay-backend/internal/account/repository"
)

// EntryInput 单条流水输入。
type EntryInput struct {
	WalletID       uint64
	Direction      string // DEBIT / CREDIT
	Amount         int64
	BalanceAfter   int64
	BizType        string // PAYMENT / SPLIT / REFUND / WITHDRAW / ADJUST
	BizID          string
	CounterpartyID *uint64
	IdempotencyKey string
	TraceID        string
	Remark         string
}

// TransferRequest 转账请求（一借一贷）。
type TransferRequest struct {
	FromWalletID uint64
	ToWalletID   uint64
	Amount       int64
	BizType      string
	BizID        string
	TraceID      string
}

// Service 账本服务。
type Service struct {
	walletRepo  *repository.WalletRepo
	journalRepo *repository.JournalRepo
	logger      *zap.Logger
}

// NewService 构造 Service。
func NewService(wr *repository.WalletRepo, jr *repository.JournalRepo, logger *zap.Logger) *Service {
	return &Service{walletRepo: wr, journalRepo: jr, logger: logger}
}

// Transfer 转账：在单事务内完成"扣减 + 入账 + 双流水"，带幂等。
func (s *Service) Transfer(ctx context.Context, req *TransferRequest) error {
	if req.Amount <= 0 {
		return errors.New("amount must be positive")
	}
	if req.FromWalletID == req.ToWalletID {
		return errors.New("from == to")
	}
	idemKey := fmt.Sprintf("%s:%s:%d:%d", req.BizType, req.BizID, req.FromWalletID, req.ToWalletID)

	return s.journalRepo.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 1) 扣减方
		from, err := s.walletRepo.GetByID(ctx, req.FromWalletID)
		if err != nil {
			return err
		}
		if from.Balance < req.Amount {
			prom.WalletBalanceMismatch.Inc()
			return fmt.Errorf("insufficient balance: wallet=%d have=%d need=%d", from.ID, from.Balance, req.Amount)
		}
		if err := s.walletRepo.UpdateBalance(ctx, from.ID, from.Version, from.Balance-req.Amount); err != nil {
			return err
		}
		// 2) 贷方流水
		if err := s.appendWithTx(ctx, tx, &EntryInput{
			WalletID:       from.ID,
			Direction:      "DEBIT",
			Amount:         req.Amount,
			BalanceAfter:   from.Balance - req.Amount,
			BizType:        req.BizType,
			BizID:          req.BizID,
			CounterpartyID: &req.ToWalletID,
			IdempotencyKey: idemKey,
			TraceID:        req.TraceID,
			Remark:         "transfer debit",
		}); err != nil {
			return err
		}

		// 3) 入账方
		to, err := s.walletRepo.GetByID(ctx, req.ToWalletID)
		if err != nil {
			return err
		}
		if err := s.walletRepo.UpdateBalance(ctx, to.ID, to.Version, to.Balance+req.Amount); err != nil {
			return err
		}
		// 4) 借方流水
		return s.appendWithTx(ctx, tx, &EntryInput{
			WalletID:       to.ID,
			Direction:      "CREDIT",
			Amount:         req.Amount,
			BalanceAfter:   to.Balance + req.Amount,
			BizType:        req.BizType,
			BizID:          req.BizID,
			CounterpartyID: &req.FromWalletID,
			IdempotencyKey: idemKey + ":c",
			TraceID:        req.TraceID,
			Remark:         "transfer credit",
		})
	})
}

// appendWithTx 在事务内追加流水。
func (s *Service) appendWithTx(ctx context.Context, tx *gorm.DB, in *EntryInput) error {
	m := &repository.JournalEntryModel{
		ID:             uuid.NewString(),
		WalletID:       in.WalletID,
		Direction:      in.Direction,
		Amount:         in.Amount,
		BalanceAfter:   in.BalanceAfter,
		BizType:        in.BizType,
		BizID:          in.BizID,
		CounterpartyID: in.CounterpartyID,
		IdempotencyKey: in.IdempotencyKey,
		TraceID:        in.TraceID,
		Remark:         in.Remark,
	}
	return tx.WithContext(ctx).Create(m).Error
}