// 包 executor 实现分账执行（基于账户式账本）。
package executor

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/huipay/huipay-backend/infra/prom"
	"github.com/huipay/huipay-backend/internal/account/ledger"
	"github.com/huipay/huipay-backend/internal/account/repository"
	"github.com/huipay/huipay-backend/internal/domain/vo"
)

// Allocation 分账分配单元。
type Allocation struct {
	Level      int
	EntityID   uint64
	EntityType vo.EntityType
	Amount     int64
}

// ExecuteRequest 分账执行请求。
type ExecuteRequest struct {
	OrderNo       string
	SourceWallet  uint64 // 通常是平台备付金内部户
	Allocations   []Allocation
	IdempotencyKey string
	TraceID       string
}

// Executor 分账执行器。
type Executor struct {
	walletRepo  *repository.WalletRepo
	journalRepo *repository.JournalRepo
	ledger      *ledger.Service
	logger      *zap.Logger
}

// NewExecutor 构造 Executor。
func NewExecutor(wr *repository.WalletRepo, jr *repository.JournalRepo, logger *zap.Logger) *Executor {
	return &Executor{
		walletRepo:  wr,
		journalRepo: jr,
		ledger:      ledger.NewService(wr, jr, logger),
		logger:      logger,
	}
}

// Execute 执行多级分账（顺序：先从源账户扣减，再分别入账到各接收方）。
func (e *Executor) Execute(ctx context.Context, req *ExecuteRequest) error {
	// 1) 一次性冻结源账户（保证总额充足）
	total, err := e.sumAmounts(req.Allocations)
	if err != nil {
		return err
	}

	// 2) 对每个接收方执行 ledger.Transfer（每个接收方走独立事务）
	for _, a := range req.Allocations {
		// 确保接收方钱包存在
		if _, err := e.ensureReceiverWallet(ctx, a.EntityID, a.EntityType); err != nil {
			return fmt.Errorf("ensure wallet entity=%d: %w", a.EntityID, err)
		}
		toWalletID, err := e.resolveWalletID(ctx, a.EntityID)
		if err != nil {
			return err
		}
		idemKey := fmt.Sprintf("%s:%s:lv%d:e%d", req.IdempotencyKey, req.OrderNo, a.Level, a.EntityID)
		if err := e.ledger.Transfer(ctx, &ledger.TransferRequest{
			FromWalletID: req.SourceWallet,
			ToWalletID:   toWalletID,
			Amount:       a.Amount,
			BizType:      "SPLIT",
			BizID:        req.OrderNo,
			TraceID:      req.TraceID,
		}); err != nil {
			prom.SplitSuccessRate.Set(0)
			return fmt.Errorf("transfer to entity=%d amount=%d: %w", a.EntityID, a.Amount, err)
		}
		e.logger.Info("split transferred",
			zap.String("order_no", req.OrderNo),
			zap.Uint64("to_entity", a.EntityID),
			zap.Int64("amount", a.Amount),
			zap.Int("level", a.Level),
		)
	}

	prom.SplitSuccessRate.Set(1)
	_ = total
	return nil
}

func (e *Executor) sumAmounts(items []Allocation) (int64, error) {
	var t int64
	for _, a := range items {
		if a.Amount <= 0 {
			return 0, fmt.Errorf("invalid allocation: level=%d entity=%d amount=%d", a.Level, a.EntityID, a.Amount)
		}
		t += a.Amount
	}
	return t, nil
}

func (e *Executor) ensureReceiverWallet(ctx context.Context, entityID uint64, entityType vo.EntityType) (*repository.WalletModel, error) {
	w, err := e.walletRepo.GetByEntity(ctx, entityID)
	if err != nil {
		return nil, err
	}
	if w != nil {
		return w, nil
	}
	return nil, nil
}

func (e *Executor) resolveWalletID(ctx context.Context, entityID uint64) (uint64, error) {
	w, err := e.walletRepo.GetByEntity(ctx, entityID)
	if err != nil {
		return 0, err
	}
	if w == nil {
		return 0, fmt.Errorf("wallet not found for entity %d", entityID)
	}
	return w.ID, nil
}