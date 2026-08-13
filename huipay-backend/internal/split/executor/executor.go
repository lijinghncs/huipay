// 包 executor 实现分账执行（基于账户式账本）。
package executor

import (
	"context"
	"encoding/binary"
	"fmt"
	"hash/fnv"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm/clause"

	"github.com/huipay/huipay-backend/infra/prom"
	"github.com/huipay/huipay-backend/internal/account/ledger"
	"github.com/huipay/huipay-backend/internal/account/repository"
	"github.com/huipay/huipay-backend/internal/domain/vo"
	"github.com/huipay/huipay-backend/internal/payment/channel"
	"github.com/huipay/huipay-backend/internal/payment/router"
)

// maxAttempts 通道调用与内部转账的最大尝试次数。
const maxAttempts = 3

// Allocation 分账分配单元。
type Allocation struct {
	Level      int
	EntityID   uint64
	EntityType vo.EntityType
	Amount     int64
}

// SplitExecutionModel 分账执行记录（t_split_execution）。
type SplitExecutionModel struct {
	ID               string     `gorm:"column:id;primaryKey"`
	OrderNo          string     `gorm:"column:order_no;size:32;not null"`
	StoreID          *uint64    `gorm:"column:store_id"`
	RuleID           *uint64    `gorm:"column:rule_id"`
	ReceiverEntityID uint64     `gorm:"column:receiver_entity_id;not null"`
	ReceiverType     string     `gorm:"column:receiver_type;size:32;not null"`
	Amount           int64      `gorm:"column:amount;not null"`
	Level            int        `gorm:"column:level;not null;default:1"`
	Channel          string     `gorm:"column:channel;size:32"`
	ChannelSplitNo   string     `gorm:"column:channel_split_no;size:64"`
	Status           string     `gorm:"column:status;size:16;not null"`
	RetryCount       int        `gorm:"column:retry_count;not null;default:0"`
	LastError        string     `gorm:"column:last_error;size:512"`
	ExecutedAt       *time.Time `gorm:"column:executed_at"`
}

// TableName 表名。
func (SplitExecutionModel) TableName() string { return "t_split_execution" }

// ExecuteRequest 分账执行请求。
type ExecuteRequest struct {
	OrderNo       string
	SourceWallet  uint64 // 通常是平台备付金内部户
	Allocations   []Allocation
	StoreID       uint64 // 关联门店 ID（可选）
	RuleID        uint64 // 命中规则 ID（可选）
	Channel       vo.ChannelCode
	IdempotencyKey string
	TraceID        string
}

// Executor 分账执行器。
type Executor struct {
	walletRepo  *repository.WalletRepo
	journalRepo *repository.JournalRepo
	ledger      *ledger.Service
	channels    *router.Router
	logger      *zap.Logger
}

// NewExecutor 构造 Executor。
func NewExecutor(wr *repository.WalletRepo, jr *repository.JournalRepo, channels *router.Router, logger *zap.Logger) *Executor {
	return &Executor{
		walletRepo:  wr,
		journalRepo: jr,
		ledger:      ledger.NewService(wr, jr, logger),
		channels:    channels,
		logger:      logger,
	}
}

// Execute 执行多级分账（顺序：先从源账户扣减，再分别入账到各接收方）。
// 幂等：已成功分账的接收方跳过（支持部分失败后重入）；通道调用与内部转账具备幂等保护，可安全重试。
func (e *Executor) Execute(ctx context.Context, req *ExecuteRequest) error {
	if _, err := e.sumAmounts(req.Allocations); err != nil {
		return err
	}
	adapter := e.resolveAdapter(req.Channel)

	for _, a := range req.Allocations {
		// 幂等检查：该接收方已成功分账则跳过
		done, err := e.hasSuccess(ctx, req.OrderNo, a.EntityID)
		if err != nil {
			return err
		}
		if done {
			e.logger.Info("receiver already split, skip",
				zap.String("order_no", req.OrderNo), zap.Uint64("entity", a.EntityID))
			continue
		}

		if _, err := e.ensureReceiverWallet(ctx, a.EntityID, a.EntityType); err != nil {
			return fmt.Errorf("ensure wallet entity=%d: %w", a.EntityID, err)
		}
		toWalletID, err := e.resolveWalletID(ctx, a.EntityID, a.EntityType)
		if err != nil {
			return err
		}

		// 1) 通道分账（带重试；无可用通道时跳过，仅本地入账）
		channelSplitNo, err := e.splitWithRetry(ctx, req, a, adapter)
		if err != nil {
			_ = e.recordExecutionStatus(ctx, req, a, "FAILED", err.Error(), "", maxAttempts)
			prom.SplitSuccessRate.Set(0)
			return fmt.Errorf("channel split entity=%d amount=%d: %w", a.EntityID, a.Amount, err)
		}

		// 2) 内部转账（带重试；ledger 幂等键保护，不会重复入账）
		if err := e.transferWithRetry(ctx, req, a, toWalletID); err != nil {
			_ = e.recordExecutionStatus(ctx, req, a, "FAILED", err.Error(), channelSplitNo, maxAttempts)
			prom.SplitSuccessRate.Set(0)
			return fmt.Errorf("transfer to entity=%d amount=%d: %w", a.EntityID, a.Amount, err)
		}

		// 3) 记录成功执行状态，回填通道分账单号
		if err := e.recordExecutionStatus(ctx, req, a, "SUCCESS", "", channelSplitNo, 0); err != nil {
			e.logger.Warn("record split execution fail",
				zap.String("order_no", req.OrderNo), zap.Error(err))
		}
		e.logger.Info("split transferred",
			zap.String("order_no", req.OrderNo),
			zap.Uint64("to_entity", a.EntityID),
			zap.Int64("amount", a.Amount),
			zap.Int("level", a.Level),
			zap.String("channel_split_no", channelSplitNo),
		)
	}

	prom.SplitSuccessRate.Set(1)
	return nil
}

// ListByOrderNo 查询某订单的全部分账执行记录。
func (e *Executor) ListByOrderNo(ctx context.Context, orderNo string) ([]SplitExecutionModel, error) {
	var rows []SplitExecutionModel
	if err := e.journalRepo.DB().WithContext(ctx).
		Where("order_no = ?", orderNo).
		Order("level ASC").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// SplitExecutionSummary 分账记录列表行（按订单聚合）。
type SplitExecutionSummary struct {
	OrderNo       string     `json:"order_no"`
	MerchantName  string     `json:"merchant_name"`
	TotalAmount   int64      `json:"total_amount"`   // 分账总额（分）
	ReceiverCount int64      `json:"receiver_count"` // 接收方数
	Status        string     `json:"status"`         // SUCCESS / PARTIAL / FAILED
	Channel       string     `json:"channel"`
	ExecutedAt    *time.Time `json:"executed_at"`
}

// ListByMerchant 按商户分页查询分账记录（JOIN t_order 过滤商户，按订单聚合）。
func (e *Executor) ListByMerchant(ctx context.Context, merchantID uint64, offset, limit int) ([]SplitExecutionSummary, int64, error) {
	db := e.journalRepo.DB().WithContext(ctx)

	var total int64
	if err := db.Raw(`SELECT COUNT(DISTINCT se.order_no)
		FROM t_split_execution se
		JOIN t_order o ON o.order_no = se.order_no
		WHERE o.merchant_id = ?`, merchantID).Scan(&total).Error; err != nil {
		return nil, 0, err
	}

	type row struct {
		OrderNo       string
		MerchantName  string
		TotalAmount   int64
		TotalCount    int64
		SuccessCount  int64
		FailedCount   int64
		Channel       string
		ExecutedAt    *time.Time
	}
	var rows []row
	if err := db.Raw(`SELECT
			se.order_no,
			en.name AS merchant_name,
			COALESCE(SUM(se.amount), 0) AS total_amount,
			COUNT(*) AS total_count,
			SUM(CASE WHEN se.status = 'SUCCESS' THEN 1 ELSE 0 END) AS success_count,
			SUM(CASE WHEN se.status = 'FAILED' THEN 1 ELSE 0 END) AS failed_count,
			MAX(se.channel) AS channel,
			MAX(se.executed_at) AS executed_at
		FROM t_split_execution se
		JOIN t_order o ON o.order_no = se.order_no
		LEFT JOIN t_entity en ON en.id = o.merchant_id
		WHERE o.merchant_id = ?
		GROUP BY se.order_no, en.name
		ORDER BY executed_at DESC
		LIMIT ? OFFSET ?`, merchantID, limit, offset).Scan(&rows).Error; err != nil {
		return nil, 0, err
	}

	out := make([]SplitExecutionSummary, 0, len(rows))
	for _, r := range rows {
		status := "SUCCESS"
		switch {
		case r.TotalCount > 0 && r.FailedCount == r.TotalCount:
			status = "FAILED"
		case r.SuccessCount != r.TotalCount:
			status = "PARTIAL"
		}
		out = append(out, SplitExecutionSummary{
			OrderNo:       r.OrderNo,
			MerchantName:  r.MerchantName,
			TotalAmount:   r.TotalAmount,
			ReceiverCount: r.TotalCount,
			Status:        status,
			Channel:       r.Channel,
			ExecutedAt:    r.ExecutedAt,
		})
	}
	return out, total, nil
}

// SplitExecutionDetail 分账明细行（含接收方名称）。
type SplitExecutionDetail struct {
	ReceiverEntityID uint64     `json:"receiver_entity_id"`
	ReceiverType     string     `json:"receiver_type"`
	ReceiverName     string     `json:"receiver_name"`
	Amount           int64      `json:"amount"`
	Level            int        `json:"level"`
	Status           string     `json:"status"`
	ChannelSplitNo   string     `json:"channel_split_no"`
	RetryCount       int        `json:"retry_count"`
	LastError        string     `json:"last_error"`
	ExecutedAt       *time.Time `json:"executed_at"`
}

// ListByOrderNoWithReceiver 按订单查询分账明细并回填接收方名称（校验订单归属商户）。
// 返回 nil,nil 表示订单不存在或不属于该商户。
func (e *Executor) ListByOrderNoWithReceiver(ctx context.Context, merchantID uint64, orderNo string) ([]SplitExecutionDetail, error) {
	db := e.journalRepo.DB().WithContext(ctx)

	var owner int64
	if err := db.Raw(`SELECT COUNT(*) FROM t_order WHERE order_no = ? AND merchant_id = ?`, orderNo, merchantID).Scan(&owner).Error; err != nil {
		return nil, err
	}
	if owner == 0 {
		return nil, nil
	}

	type row struct {
		ReceiverEntityID uint64
		ReceiverType     string
		Amount           int64
		Level            int
		Status           string
		ChannelSplitNo   string
		RetryCount       int
		LastError        string
		ExecutedAt       *time.Time
		StoreName        string // STORE 接收方名称
		MerchantName     string // MERCHANT 接收方名称
	}
	var rows []row
	if err := db.Raw(`SELECT
			se.receiver_entity_id,
			se.receiver_type,
			se.amount,
			se.level,
			se.status,
			se.channel_split_no,
			se.retry_count,
			se.last_error,
			se.executed_at,
			st.name AS store_name,
			e.name AS merchant_name
		FROM t_split_execution se
		LEFT JOIN t_store st ON se.receiver_type = 'STORE' AND st.id = se.receiver_entity_id
		LEFT JOIN t_entity e ON se.receiver_type = 'MERCHANT' AND e.id = se.receiver_entity_id
		WHERE se.order_no = ?
		ORDER BY se.level ASC`, orderNo).Scan(&rows).Error; err != nil {
		return nil, err
	}

	out := make([]SplitExecutionDetail, 0, len(rows))
	for _, r := range rows {
		name := fmt.Sprintf("#%d", r.ReceiverEntityID)
		if r.ReceiverType == "STORE" && r.StoreName != "" {
			name = r.StoreName
		} else if r.ReceiverType == "MERCHANT" && r.MerchantName != "" {
			name = r.MerchantName
		}
		out = append(out, SplitExecutionDetail{
			ReceiverEntityID: r.ReceiverEntityID,
			ReceiverType:     r.ReceiverType,
			ReceiverName:     name,
			Amount:           r.Amount,
			Level:            r.Level,
			Status:           r.Status,
			ChannelSplitNo:   r.ChannelSplitNo,
			RetryCount:       r.RetryCount,
			LastError:        r.LastError,
			ExecutedAt:       r.ExecutedAt,
		})
	}
	return out, nil
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

// hasSuccess 判断某订单的某接收方是否已成功分账。
func (e *Executor) hasSuccess(ctx context.Context, orderNo string, entityID uint64) (bool, error) {
	var count int64
	err := e.journalRepo.DB().WithContext(ctx).
		Model(&SplitExecutionModel{}).
		Where("order_no = ? AND receiver_entity_id = ? AND status = ?", orderNo, entityID, "SUCCESS").
		Count(&count).Error
	return count > 0, err
}

// splitWithRetry 调用通道分账接口，失败重试 maxAttempts 次。
func (e *Executor) splitWithRetry(ctx context.Context, req *ExecuteRequest, a Allocation, adapter channel.Adapter) (string, error) {
	if adapter == nil {
		return "", nil // 无可用通道时跳过通道调用，仅本地入账
	}
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		resp, err := adapter.Split(ctx, &channel.SplitRequest{
			OrderNo:   req.OrderNo,
			Receivers: []channel.Receiver{{EntityID: a.EntityID, Amount: a.Amount}},
		})
		if err == nil {
			return resp.ChannelSplitNo, nil
		}
		lastErr = err
		e.logger.Warn("channel split attempt fail",
			zap.String("order_no", req.OrderNo), zap.Uint64("entity", a.EntityID),
			zap.Int("attempt", attempt), zap.Int("max", maxAttempts), zap.Error(err))
	}
	return "", lastErr
}

// transferWithRetry 执行内部转账，失败重试 maxAttempts 次。
func (e *Executor) transferWithRetry(ctx context.Context, req *ExecuteRequest, a Allocation, toWalletID uint64) error {
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		err := e.ledger.Transfer(ctx, &ledger.TransferRequest{
			FromWalletID: req.SourceWallet,
			ToWalletID:   toWalletID,
			Amount:       a.Amount,
			BizType:      "SPLIT",
			BizID:        req.OrderNo,
			TraceID:      req.TraceID,
		})
		if err == nil {
			return nil
		}
		lastErr = err
		e.logger.Warn("split transfer attempt fail",
			zap.String("order_no", req.OrderNo), zap.Uint64("entity", a.EntityID),
			zap.Int("attempt", attempt), zap.Int("max", maxAttempts), zap.Error(err))
	}
	return lastErr
}

// recordExecutionStatus 写分账执行记录（含门店、规则、通道、重试信息）；已存在时按 (order_no, receiver) 更新。
func (e *Executor) recordExecutionStatus(ctx context.Context, req *ExecuteRequest, a Allocation, status, lastErr, channelSplitNo string, retryCount int) error {
	var storeID, ruleID *uint64
	if req.StoreID > 0 {
		id := req.StoreID
		storeID = &id
	}
	if req.RuleID > 0 {
		id := req.RuleID
		ruleID = &id
	}
	now := time.Now()
	m := &SplitExecutionModel{
		ID:               executionID(req.OrderNo, a.EntityID),
		OrderNo:          req.OrderNo,
		StoreID:          storeID,
		RuleID:           ruleID,
		ReceiverEntityID: a.EntityID,
		ReceiverType:     string(a.EntityType),
		Amount:           a.Amount,
		Level:            a.Level,
		Channel:          string(req.Channel),
		ChannelSplitNo:   channelSplitNo,
		Status:           status,
		RetryCount:       retryCount,
		LastError:        lastErr,
		ExecutedAt:       &now,
	}
	return e.journalRepo.DB().WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "order_no"}, {Name: "receiver_entity_id"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"status", "channel", "channel_split_no", "retry_count", "last_error", "executed_at", "level", "amount",
			}),
		}).
		Create(m).Error
}

// executionID 由订单号 + 接收方生成 20 位确定性 ID，适配 t_split_execution.id CHAR(20)。
func executionID(orderNo string, entityID uint64) string {
	h := fnv.New64a()
	_, _ = h.Write([]byte(orderNo))
	_ = binary.Write(h, binary.BigEndian, entityID)
	return fmt.Sprintf("%020d", h.Sum64())
}

func (e *Executor) resolveAdapter(code vo.ChannelCode) channel.Adapter {
	if e.channels == nil {
		return nil
	}
	if code != "" {
		if a := e.channels.GetAdapter(code); a != nil {
			return a
		}
	}
	return e.channels.GetAdapter(vo.ChannelWeChat)
}

func (e *Executor) ensureReceiverWallet(ctx context.Context, entityID uint64, entityType vo.EntityType) (*repository.WalletModel, error) {
	// 分账接收方为门店，按 (entity_id, entity_type) 定位，避免与商户/通道户 id 冲突；不存在则自动开通。
	w, err := e.walletRepo.GetByEntityType(ctx, entityID, string(entityType))
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
	if err := e.walletRepo.DB().WithContext(ctx).Create(m).Error; err != nil {
		// 并发创建唯一键冲突时重查
		w2, _ := e.walletRepo.GetByEntityType(ctx, entityID, string(entityType))
		if w2 != nil {
			return w2, nil
		}
		return nil, fmt.Errorf("create wallet: %w", err)
	}
	return m, nil
}

func (e *Executor) resolveWalletID(ctx context.Context, entityID uint64, entityType vo.EntityType) (uint64, error) {
	w, err := e.walletRepo.GetByEntityType(ctx, entityID, string(entityType))
	if err != nil {
		return 0, err
	}
	if w == nil {
		return 0, fmt.Errorf("wallet not found for entity %d type %s", entityID, entityType)
	}
	return w.ID, nil
}