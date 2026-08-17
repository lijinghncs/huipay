// 包 executor 实现分账执行（基于账户式账本）。
package executor

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm/clause"

	"github.com/huipay/huipay-backend/infra/notify"
	"github.com/huipay/huipay-backend/infra/prom"
	"github.com/huipay/huipay-backend/internal/account/ledger"
	"github.com/huipay/huipay-backend/internal/account/repository"
	splitrepo "github.com/huipay/huipay-backend/internal/split/repository"
	"github.com/huipay/huipay-backend/internal/domain/vo"
	"github.com/huipay/huipay-backend/internal/payment/channel"
	"github.com/huipay/huipay-backend/internal/payment/router"
)

// maxAttempts 通道调用与内部转账的最大尝试次数。
const maxAttempts = 3

// 分账模式常量（C1 层：通道降级闸门，存 t_entity.split_mode）。
const (
	SplitModeAuto            = "AUTO"             // 有通道走通道；无通道降级本地入账并标记 degraded
	SplitModeLocalOnly       = "LOCAL_ONLY"       // 仅本地记账（不调通道，标记 degraded）
	SplitModeChannelRequired = "CHANNEL_REQUIRED" // 通道不可用即失败，不本地入账
)

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
	ChannelReqNo     string     `gorm:"column:channel_req_no;size:64"`
	StoreID          *uint64    `gorm:"column:store_id"`
	RuleID           *uint64    `gorm:"column:rule_id"`
	ReceiverEntityID uint64     `gorm:"column:receiver_entity_id;not null"`
	ReceiverType     string     `gorm:"column:receiver_type;size:32;not null"`
	Amount           int64      `gorm:"column:amount;not null"`
	Level            int        `gorm:"column:level;not null;default:1"`
	Channel          string     `gorm:"column:channel;size:32"`
	ChannelSplitNo   string     `gorm:"column:channel_split_no;size:64"`
	Degraded         int        `gorm:"column:degraded;not null;default:0"`
	Status           string     `gorm:"column:status;size:16;not null"`
	RetryCount       int        `gorm:"column:retry_count;not null;default:0"`
	LastError        string     `gorm:"column:last_error;size:512"`
	ExecutedAt       *time.Time `gorm:"column:executed_at"`
}

// TableName 表名。
func (SplitExecutionModel) TableName() string { return "t_split_execution" }

// ExecuteRequest 分账执行请求。
type ExecuteRequest struct {
	MerchantID     uint64
	OrderNo        string
	SourceWallet   uint64 // 通常是平台备付金内部户
	Allocations    []Allocation
	StoreID        uint64 // 关联门店 ID（可选）
	RuleID         uint64 // 命中规则 ID（可选）
	Channel        vo.ChannelCode
	IdempotencyKey string
	TraceID        string
}

// Executor 分账执行器。
type Executor struct {
	walletRepo      *repository.WalletRepo
	journalRepo     *repository.JournalRepo
	orderStatusRepo *splitrepo.SplitOrderStatusRepo
	ledger          *ledger.Service
	channels        *router.Router
	alerter         notify.Alerter
	logger          *zap.Logger
}

// NewExecutor 构造 Executor。
func NewExecutor(wr *repository.WalletRepo, jr *repository.JournalRepo, osr *splitrepo.SplitOrderStatusRepo, channels *router.Router, logger *zap.Logger) *Executor {
	return &Executor{
		walletRepo:      wr,
		journalRepo:     jr,
		orderStatusRepo: osr,
		ledger:          ledger.NewService(wr, jr, logger),
		channels:        channels,
		logger:          logger,
	}
}

// SetAlerter 注入告警器（可选；不注入则告警为空操作）。
func (e *Executor) SetAlerter(a notify.Alerter) {
	if a == nil {
		a = notify.NoopAlerter{}
	}
	e.alerter = a
}

// alert 安全触发告警（未注入时为空操作）。
func (e *Executor) alert(ctx context.Context, title, content string) {
	if e.alerter != nil {
		e.alerter.Alert(ctx, title, content)
	}
}

// Execute 执行多级分账（顺序：先从源账户扣减，再分别入账到各接收方）。
// 容错：C2 余额预校验防部分成功；A1 通道幂等单号防通道侧重复；A2/A4 订单级状态+快照供重试；C1 通道未配置标记降级。
func (e *Executor) Execute(ctx context.Context, req *ExecuteRequest) error {
	// 过滤已成功接收方（幂等重入）
	pending := make([]Allocation, 0, len(req.Allocations))
	for _, a := range req.Allocations {
		done, err := e.hasSuccess(ctx, req.OrderNo, a.EntityID)
		if err != nil {
			return err
		}
		if done {
			e.logger.Info("receiver already split, skip",
				zap.String("order_no", req.OrderNo), zap.Uint64("entity", a.EntityID))
			continue
		}
		pending = append(pending, a)
	}
	if len(pending) == 0 {
		// 全部已成功，幂等完成
		return e.finalizeOrderStatus(ctx, req, "" , "")
	}

	pendingSum, err := e.sumAmounts(pending)
	if err != nil {
		return err
	}

	// C2 余额预校验：待分金额（未成功接收方和）须 ≤ 商户钱包余额，否则整体失败，避免部分成功
	if err := e.checkBalance(ctx, req.SourceWallet, pendingSum); err != nil {
		e.incFailure("insufficient_balance")
		return e.finalizeOrderStatus(ctx, req, "", err.Error())
	}

	// C1 分账模式闸门：AUTO 自动降级 / LOCAL_ONLY 仅本地 / CHANNEL_REQUIRED 强制通道
	mode := e.getSplitMode(ctx, req.MerchantID)
	adapter := e.resolveAdapter(req.Channel)
	degraded := 0
	switch mode {
	case SplitModeLocalOnly:
		adapter = nil
		degraded = 1 // LOCAL_ONLY：仅本地记账并标记降级
	case SplitModeChannelRequired:
		if adapter == nil {
			// 强制通道但通道不可用：整体失败不本地入账，进入重试队列
			e.incFailure("channel_fail")
			return e.finalizeOrderStatus(ctx, req, "", "channel required but not configured")
		}
	default: // AUTO
		if adapter == nil {
			degraded = 1 // 通道未配置：仅本地入账并标记降级
		}
	}

	// A2/A4 订单级状态：PROCESSING（写入分配快照）
	if err := e.upsertOrderStatus(ctx, req, pendingSum, degraded); err != nil {
		return err
	}

	successCount := 0
	for _, a := range pending {
		if _, err := e.ensureReceiverWallet(ctx, a.EntityID, a.EntityType); err != nil {
			return e.finalizeOrderStatus(ctx, req, "", fmt.Sprintf("ensure wallet entity=%d: %v", a.EntityID, err))
		}
		toWalletID, err := e.resolveWalletID(ctx, a.EntityID, a.EntityType)
		if err != nil {
			return e.finalizeOrderStatus(ctx, req, "", err.Error())
		}

		// A1 通道幂等单号：确定性生成并持久化，重试复用
		channelReqNo := channelReqNo(req.OrderNo, a.EntityID)

		// 1) 通道分账（带重试；无可用通道时跳过，仅本地入账）
		channelSplitNo, err := e.splitWithRetry(ctx, req, a, adapter, channelReqNo)
		if err != nil {
			_ = e.recordExecutionStatus(ctx, req, a, "FAILED", err.Error(), "", maxAttempts, degraded, channelReqNo)
			e.incFailure("channel_fail")
			return e.finalizeOrderStatus(ctx, req, "", fmt.Sprintf("channel split entity=%d: %v", a.EntityID, err))
		}

		// 2) 内部转账（带重试；ledger 幂等键保护，不会重复入账）
		if err := e.transferWithRetry(ctx, req, a, toWalletID); err != nil {
			_ = e.recordExecutionStatus(ctx, req, a, "FAILED", err.Error(), channelSplitNo, maxAttempts, degraded, channelReqNo)
			e.incFailure("transfer_fail")
			return e.finalizeOrderStatus(ctx, req, "", fmt.Sprintf("transfer to entity=%d: %v", a.EntityID, err))
		}

		// 3) 记录成功执行状态，回填通道分账单号
		if err := e.recordExecutionStatus(ctx, req, a, "SUCCESS", "", channelSplitNo, 0, degraded, channelReqNo); err != nil {
			e.logger.Warn("record split execution fail",
				zap.String("order_no", req.OrderNo), zap.Error(err))
		}
		successCount++
		prom.SplitAmountTotal.Add(float64(a.Amount))
		e.logger.Info("split transferred",
			zap.String("order_no", req.OrderNo),
			zap.Uint64("to_entity", a.EntityID),
			zap.Int64("amount", a.Amount),
			zap.Int("level", a.Level),
			zap.String("channel_split_no", channelSplitNo),
			zap.String("channel_req_no", channelReqNo),
		)
	}

	return e.finalizeOrderStatus(ctx, req, "", "")
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
	RuleID        uint64     `json:"rule_id"`
	RuleName      string     `json:"rule_name"`
	TotalAmount   int64      `json:"total_amount"`   // 分账总额（分）
	ReceiverCount int64      `json:"receiver_count"` // 接收方数
	Status        string     `json:"status"`         // SUCCESS / PARTIAL / FAILED
	Channel       string     `json:"channel"`
	ExecutedAt    *time.Time `json:"executed_at"`
}

// SplitExecutionFilter 分账记录列表过滤条件。
type SplitExecutionFilter struct {
	Status string    // SUCCESS / PARTIAL / FAILED（空表示全部）
	Start  time.Time // 执行时间下限（可选）
	End    time.Time // 执行时间上限（可选）
	RuleID uint64    // 命中规则 ID（可选）
}

// ListByMerchant 按商户分页查询分账记录（JOIN t_order 过滤商户，按订单聚合）。
func (e *Executor) ListByMerchant(ctx context.Context, merchantID uint64, offset, limit int, f SplitExecutionFilter) ([]SplitExecutionSummary, int64, error) {
	db := e.journalRepo.DB().WithContext(ctx)

	where := "(o.merchant_id = ? OR sb.merchant_id = ?)"
	args := []any{merchantID, merchantID}
	if !f.Start.IsZero() {
		where += " AND se.executed_at >= ?"
		args = append(args, f.Start)
	}
	if !f.End.IsZero() {
		where += " AND se.executed_at <= ?"
		args = append(args, f.End)
	}
	if f.RuleID > 0 {
		where += " AND se.rule_id = ?"
		args = append(args, f.RuleID)
	}

	having := ""
	switch f.Status {
	case "SUCCESS":
		having = " HAVING success_count = total_count"
	case "FAILED":
		having = " HAVING failed_count = total_count"
	case "PARTIAL":
		having = " HAVING success_count <> total_count AND failed_count <> total_count"
	}

	// 总数与列表共用同一 WHERE + HAVING，保证状态过滤下 total 与 items 一致
	var total int64
	totalQuery := `SELECT COUNT(*) FROM (
		SELECT se.order_no,
			COUNT(*) AS total_count,
			SUM(CASE WHEN se.status = 'SUCCESS' THEN 1 ELSE 0 END) AS success_count,
			SUM(CASE WHEN se.status = 'FAILED' THEN 1 ELSE 0 END) AS failed_count
		FROM t_split_execution se
		LEFT JOIN t_order o ON o.order_no = se.order_no
		LEFT JOIN t_split_bill sb ON sb.batch_no = se.order_no
		WHERE ` + where + `
		GROUP BY se.order_no` + having + `
	) t`
	if err := db.Raw(totalQuery, args...).Scan(&total).Error; err != nil {
		return nil, 0, err
	}

	type row struct {
		OrderNo       string
		MerchantName  string
		RuleID        uint64
		RuleName      string
		TotalAmount   int64
		TotalCount    int64
		SuccessCount  int64
		FailedCount   int64
		Channel       string
		ExecutedAt    *time.Time
	}
	var rows []row
	query := `SELECT
			se.order_no,
			en.name AS merchant_name,
			COALESCE(MAX(se.rule_id), 0) AS rule_id,
			COALESCE(MAX(sr.rule_name), '') AS rule_name,
			COALESCE(SUM(se.amount), 0) AS total_amount,
			COUNT(*) AS total_count,
			SUM(CASE WHEN se.status = 'SUCCESS' THEN 1 ELSE 0 END) AS success_count,
			SUM(CASE WHEN se.status = 'FAILED' THEN 1 ELSE 0 END) AS failed_count,
			MAX(se.channel) AS channel,
			MAX(se.executed_at) AS executed_at
		FROM t_split_execution se
		LEFT JOIN t_order o ON o.order_no = se.order_no
		LEFT JOIN t_split_bill sb ON sb.batch_no = se.order_no
		LEFT JOIN t_entity en ON en.id = COALESCE(o.merchant_id, sb.merchant_id)
		LEFT JOIN t_split_rule sr ON sr.id = se.rule_id
		WHERE ` + where + `
		GROUP BY se.order_no, en.name` + having + `
		ORDER BY executed_at DESC
		LIMIT ? OFFSET ?`
	allArgs := append(args, limit, offset)
	if err := db.Raw(query, allArgs...).Scan(&rows).Error; err != nil {
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
			RuleID:        r.RuleID,
			RuleName:      r.RuleName,
			TotalAmount:   r.TotalAmount,
			ReceiverCount: r.TotalCount,
			Status:        status,
			Channel:       r.Channel,
			ExecutedAt:    r.ExecutedAt,
		})
	}
	return out, total, nil
}

// ListByOrderNoForMerchant 按订单查询分账执行记录，并校验归属商户（orderNo 可为真实订单号或分账单批次号；nil,nil 表示非本商户）。
func (e *Executor) ListByOrderNoForMerchant(ctx context.Context, merchantID uint64, orderNo string) ([]SplitExecutionModel, error) {
	var owner int64
	if err := e.journalRepo.DB().WithContext(ctx).Raw(
		`SELECT
			(SELECT COUNT(*) FROM t_order WHERE order_no = ? AND merchant_id = ?)
			+ (SELECT COUNT(*) FROM t_split_bill WHERE batch_no = ? AND merchant_id = ?)`,
		orderNo, merchantID, orderNo, merchantID,
	).Scan(&owner).Error; err != nil {
		return nil, err
	}
	if owner == 0 {
		return nil, nil
	}
	return e.ListByOrderNo(ctx, orderNo)
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

// ListByOrderNoWithReceiver 按订单查询分账明细并回填接收方名称（校验订单归属商户；orderNo 可为真实订单号或分账单批次号）。
// 返回 nil,nil 表示订单/批次不存在或不属于该商户。
func (e *Executor) ListByOrderNoWithReceiver(ctx context.Context, merchantID uint64, orderNo string) ([]SplitExecutionDetail, error) {
	db := e.journalRepo.DB().WithContext(ctx)

	var owner int64
	if err := db.Raw(
		`SELECT
			(SELECT COUNT(*) FROM t_order WHERE order_no = ? AND merchant_id = ?)
			+ (SELECT COUNT(*) FROM t_split_bill WHERE batch_no = ? AND merchant_id = ?)`,
		orderNo, merchantID, orderNo, merchantID,
	).Scan(&owner).Error; err != nil {
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

// splitWithRetry 调用通道分账接口，失败重试 maxAttempts 次；channelReqNo 为通道幂等单号（重试复用）。
func (e *Executor) splitWithRetry(ctx context.Context, req *ExecuteRequest, a Allocation, adapter channel.Adapter, channelReqNo string) (string, error) {
	if adapter == nil {
		return "", nil // 无可用通道时跳过通道调用，仅本地入账
	}
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		resp, err := adapter.Split(ctx, &channel.SplitRequest{
			OrderNo:      req.OrderNo,
			ChannelReqNo: channelReqNo,
			Receivers:    []channel.Receiver{{EntityID: a.EntityID, Amount: a.Amount}},
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

// recordExecutionStatus 写分账执行记录（含门店、规则、通道、通道幂等单号、降级、重试信息）；已存在时按 (order_no, receiver) 更新。
func (e *Executor) recordExecutionStatus(ctx context.Context, req *ExecuteRequest, a Allocation, status, lastErr, channelSplitNo string, retryCount, degraded int, channelReqNo string) error {
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
		ChannelReqNo:     channelReqNo,
		StoreID:          storeID,
		RuleID:           ruleID,
		ReceiverEntityID: a.EntityID,
		ReceiverType:     string(a.EntityType),
		Amount:           a.Amount,
		Level:            a.Level,
		Channel:          string(req.Channel),
		ChannelSplitNo:   channelSplitNo,
		Degraded:         degraded,
		Status:           status,
		RetryCount:       retryCount,
		LastError:        lastErr,
		ExecutedAt:       &now,
	}
	return e.journalRepo.DB().WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "order_no"}, {Name: "receiver_entity_id"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"status", "channel", "channel_split_no", "channel_req_no", "degraded", "retry_count", "last_error", "executed_at", "level", "amount",
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

// getSplitMode 读取商户分账模式（t_entity.split_mode）；缺省/异常时保守回退 AUTO。
func (e *Executor) getSplitMode(ctx context.Context, merchantID uint64) string {
	var mode string
	if err := e.journalRepo.DB().WithContext(ctx).
		Table("t_entity").Where("id = ?", merchantID).
		Pluck("split_mode", &mode).Error; err != nil {
		e.logger.Warn("get merchant split mode fail, fallback AUTO",
			zap.Uint64("merchant", merchantID), zap.Error(err))
		return SplitModeAuto
	}
	switch mode {
	case SplitModeLocalOnly, SplitModeChannelRequired:
		return mode
	default:
		return SplitModeAuto
	}
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

// checkBalance C2 余额预校验：源钱包余额须 ≥ 待分金额，避免部分成功。
func (e *Executor) checkBalance(ctx context.Context, walletID uint64, amount int64) error {
	w, err := e.walletRepo.GetByID(ctx, walletID)
	if err != nil {
		return err
	}
	if w == nil {
		return fmt.Errorf("wallet not found id=%d", walletID)
	}
	if w.Balance < amount {
		return fmt.Errorf("insufficient balance: wallet=%d balance=%d need=%d", walletID, w.Balance, amount)
	}
	return nil
}

// upsertOrderStatus A2/A4：写入/更新订单级状态（PROCESSING + 分配快照）。冲突时仅更新 status/degraded，保留快照与接收方数。
func (e *Executor) upsertOrderStatus(ctx context.Context, req *ExecuteRequest, total int64, degraded int) error {
	var ruleID *uint64
	if req.RuleID > 0 {
		id := req.RuleID
		ruleID = &id
	}
	snapshot, _ := json.Marshal(req.Allocations)
	m := &splitrepo.SplitOrderStatusModel{
		OrderNo:       req.OrderNo,
		MerchantID:    req.MerchantID,
		RuleID:        ruleID,
		RuleSnapshot:  string(snapshot),
		TotalAmount:   total,
		ReceiverCount: len(req.Allocations),
		Status:        splitrepo.OrderStatusProcessing,
		Degraded:      degraded,
	}
	return e.orderStatusRepo.DB().WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "order_no"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"status", "degraded",
			}),
		}).
		Create(m).Error
}

// finalizeOrderStatus 执行结束后回写订单级状态：统计成功数、定态、重试退避/耗尽。
// lastErr 非空表示本轮失败（部分/全部）；否则视为成功到账。
func (e *Executor) finalizeOrderStatus(ctx context.Context, req *ExecuteRequest, _ string, lastErr string) error {
	if e.orderStatusRepo == nil {
		return nil
	}
	st, err := e.orderStatusRepo.Get(ctx, req.OrderNo)
	if err != nil {
		return err
	}
	if st == nil {
		// 无状态记录（如旧数据/未走 A2 的路径），尝试补建
		snapshot, _ := json.Marshal(req.Allocations)
		st = &splitrepo.SplitOrderStatusModel{
			OrderNo:       req.OrderNo,
			MerchantID:    req.MerchantID,
			RuleSnapshot:  string(snapshot),
			TotalAmount:   req.AllocationsTotal(),
			ReceiverCount: len(req.Allocations),
		}
		if err := e.orderStatusRepo.Upsert(ctx, st); err != nil {
			return err
		}
	}

	successCount := e.countSuccess(ctx, req.OrderNo)
	receiverCount := st.ReceiverCount
	if receiverCount == 0 {
		receiverCount = len(req.Allocations)
	}
	status := splitrepo.OrderStatusSuccess
	if lastErr != "" {
		if successCount > 0 && successCount < receiverCount {
			status = splitrepo.OrderStatusPartial
		} else {
			status = splitrepo.OrderStatusFailed
		}
	}
	attempt := st.AttemptCount + 1
	var nextRetryAt *time.Time
	if status == splitrepo.OrderStatusPartial || status == splitrepo.OrderStatusFailed {
		if attempt < splitrepo.MaxRetryAttempts {
			t := time.Now().Add(splitrepo.RetryBackoff(attempt))
			nextRetryAt = &t
		} else {
			status = splitrepo.OrderStatusDead
			e.logger.Error("split order reached dead after retries",
				zap.String("order_no", req.OrderNo), zap.String("last_error", lastErr))
			// 告警：死单需人工介入（差错中心复位重开或管理端核销）
			e.alert(ctx, "【分账死单】自动重试耗尽",
				fmt.Sprintf("订单号：%s\n商户：%d\n最近错误：%s\n请前往差错中心处理", req.OrderNo, req.MerchantID, lastErr))
		}
	}

	if err := e.orderStatusRepo.UpdateResult(ctx, req.OrderNo, successCount, status, attempt, nextRetryAt, lastErr); err != nil {
		return err
	}
	e.syncOrderSplitStatus(ctx, req.OrderNo, status)
	prom.SplitOrderTotal.WithLabelValues(status).Inc()
	if status == splitrepo.OrderStatusSuccess {
		prom.SplitSuccessRate.Set(1)
	} else {
		prom.SplitSuccessRate.Set(0)
	}
	if lastErr != "" {
		e.logger.Warn("split order finalized with error",
			zap.String("order_no", req.OrderNo), zap.String("status", status), zap.Int("success", successCount))
	}
	// 非成功终态需向上层返回错误，避免服务层误报成功（部分成功也返回，交由补偿调度续跑）
	if status != splitrepo.OrderStatusSuccess {
		return fmt.Errorf("split order %s: %s", status, lastErr)
	}
	return nil
}

// syncOrderSplitStatus 分账定态后同步回写 t_order.split_status（仅终态）：
// SUCCESS -> SUCCESS；FAILED/DEAD/SUSPENDED -> FAILED；PARTIAL 不写（交由补偿续跑）。
func (e *Executor) syncOrderSplitStatus(ctx context.Context, orderNo, status string) {
	var orderSplit string
	switch status {
	case splitrepo.OrderStatusSuccess:
		orderSplit = "SUCCESS"
	case splitrepo.OrderStatusFailed, splitrepo.OrderStatusDead, splitrepo.OrderStatusSuspended:
		orderSplit = "FAILED"
	default:
		return
	}
	if err := e.orderStatusRepo.DB().WithContext(ctx).
		Table("t_order").
		Where("order_no = ?", orderNo).
		Update("split_status", orderSplit).Error; err != nil {
		e.logger.Warn("sync t_order.split_status fail", zap.String("order_no", orderNo), zap.Error(err))
	}
}

// countSuccess 统计某订单已成功分账的接收方数。
func (e *Executor) countSuccess(ctx context.Context, orderNo string) int {
	var count int64
	if err := e.journalRepo.DB().WithContext(ctx).
		Model(&SplitExecutionModel{}).
		Where("order_no = ? AND status = ?", orderNo, "SUCCESS").
		Count(&count).Error; err != nil {
		e.logger.Warn("count success fail", zap.String("order_no", orderNo), zap.Error(err))
		return 0
	}
	return int(count)
}

// channelReqNo 确定性生成通道分账幂等单号（同 (order, receiver) 恒定，重试复用防通道侧重复分账）。
func channelReqNo(orderNo string, entityID uint64) string {
	tail := orderNo
	if len(tail) > 12 {
		tail = tail[len(tail)-12:]
	}
	return fmt.Sprintf("SP%s%012d", tail, entityID)
}

// incFailure 记录失败原因指标。
func (e *Executor) incFailure(reason string) {
	prom.SplitFailureReasonTotal.WithLabelValues(reason).Inc()
}

// AllocationsTotal 计算分配总额。
func (req *ExecuteRequest) AllocationsTotal() int64 {
	var t int64
	for _, a := range req.Allocations {
		t += a.Amount
	}
	return t
}