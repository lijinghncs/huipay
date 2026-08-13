// 包 ledger 实现复式记账核心能力。
// 原则：每笔资金变动至少生成 2 条流水（借贷平衡），保证可追溯、可校验。
package ledger

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/huipay/huipay-backend/infra/prom"
	"github.com/huipay/huipay-backend/internal/account/repository"
	"github.com/huipay/huipay-backend/internal/domain/vo"
)

// journalIDSeq 流水 ID 生成用的全局递增序号（配合时间戳保证并发唯一）。
var (
	journalIDMu  sync.Mutex
	journalIDSeq int64
)

// journalID 生成 20 位十进制雪花样式 ID，适配 t_journal_entry.id CHAR(20)。
// 组成：Unix 毫秒时间戳（13 位）+ 全局递增序号（7 位），保证同毫秒内并发也不冲突。
func journalID() string {
	journalIDMu.Lock()
	journalIDSeq++
	seq := journalIDSeq
	journalIDMu.Unlock()
	return fmt.Sprintf("%013d%07d", time.Now().UnixMilli(), seq%10000000)
}

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
		ID:             journalID(),
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

// CreditFromSettlementRequest 渠道在途资金 → 商户入账（外部虚拟源户 → 商户备付金）。
type CreditFromSettlementRequest struct {
	SettlementWalletID uint64        // 通道在途资金户 wallet_id
	ToEntityID         uint64        // 商户 entity_id
	ToEntityType       vo.EntityType // 商户主体类型
	Amount             int64
	BizType            string // "PAYMENT"
	BizID              string // 订单号
	TraceID            string
}

// CreditFromSettlement 单边入账：通道在途资金户（DEBIT 流出）→ 商户备付金（CREDIT 流入）。
// 单事务内保证借贷平衡；幂等键命中时重复调用不写流水。
// 按项目约定：DEBIT=资金流出（余额减少）、CREDIT=资金流入（余额增加）。
func (s *Service) CreditFromSettlement(ctx context.Context, req *CreditFromSettlementRequest) error {
	if req.Amount <= 0 {
		return errors.New("amount must be positive")
	}
	idemKey := fmt.Sprintf("%s:%s:%d:%d", req.BizType, req.BizID, req.SettlementWalletID, req.ToEntityID)

	return s.journalRepo.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 1) 结算户扣减
		settle, err := s.walletRepo.GetByIDTx(ctx, tx, req.SettlementWalletID)
		if err != nil {
			return err
		}
		if settle.Balance < req.Amount {
			prom.WalletBalanceMismatch.Inc()
			return fmt.Errorf("insufficient settlement balance: wallet=%d have=%d need=%d", settle.ID, settle.Balance, req.Amount)
		}
		if err := s.walletRepo.UpdateBalanceTx(ctx, tx, settle.ID, settle.Version, settle.Balance-req.Amount); err != nil {
			return err
		}
		// 2) 结算户 DEBIT 流水（资金流出）
		if err := s.appendWithTx(ctx, tx, &EntryInput{
			WalletID:       settle.ID,
			Direction:      "DEBIT",
			Amount:         req.Amount,
			BalanceAfter:   settle.Balance - req.Amount,
			BizType:        req.BizType,
			BizID:          req.BizID,
			CounterpartyID: &req.ToEntityID,
			IdempotencyKey: idemKey,
			TraceID:        req.TraceID,
			Remark:         "settlement debit",
		}); err != nil {
			return err
		}

		// 3) 确保商户钱包存在
		merchant, err := s.ensureWalletTx(ctx, tx, req.ToEntityID, req.ToEntityType)
		if err != nil {
			return err
		}
		// 4) 商户入账
		if err := s.walletRepo.UpdateBalanceTx(ctx, tx, merchant.ID, merchant.Version, merchant.Balance+req.Amount); err != nil {
			return err
		}
		// 5) 商户 CREDIT 流水（资金流入）
		return s.appendWithTx(ctx, tx, &EntryInput{
			WalletID:       merchant.ID,
			Direction:      "CREDIT",
			Amount:         req.Amount,
			BalanceAfter:   merchant.Balance + req.Amount,
			BizType:        req.BizType,
			BizID:          req.BizID,
			CounterpartyID: &req.SettlementWalletID,
			IdempotencyKey: idemKey + ":c",
			TraceID:        req.TraceID,
			Remark:         "settlement credit",
		})
	})
}

// ensureWalletTx 在事务内确保主体钱包存在（创建或返回已存在），按 (entity_id, entity_type) 定位。
func (s *Service) ensureWalletTx(ctx context.Context, tx *gorm.DB, entityID uint64, entityType vo.EntityType) (*repository.WalletModel, error) {
	w, err := s.walletRepo.GetByEntityTypeTx(ctx, tx, entityID, string(entityType))
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
	if err := tx.WithContext(ctx).Create(m).Error; err != nil {
		// 并发创建时唯一索引冲突 → 重新查
		w2, _ := s.walletRepo.GetByEntityType(ctx, entityID, string(entityType))
		if w2 != nil {
			return w2, nil
		}
		return nil, fmt.Errorf("create wallet: %w", err)
	}
	return m, nil
}