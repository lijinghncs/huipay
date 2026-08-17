package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm/clause"

	"github.com/huipay/huipay-backend/infra/prom"
	"github.com/huipay/huipay-backend/internal/split/event"
	splitrepo "github.com/huipay/huipay-backend/internal/split/repository"
	"github.com/huipay/huipay-backend/internal/split/splitcfg"
	"github.com/huipay/huipay-backend/internal/split/state"
)

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
		Status:        string(state.Processing),
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
	status := state.Success
	if lastErr != "" {
		if successCount > 0 && successCount < receiverCount {
			status = state.Partial
		} else {
			status = state.Failed
		}
	}
	attempt := st.AttemptCount + 1
	var nextRetryAt *time.Time
	if status == state.Partial || status == state.Failed {
		if attempt < splitcfg.MaxRetryAttempts {
			t := time.Now().Add(splitcfg.RetryBackoff(attempt))
			nextRetryAt = &t
		} else {
			status = state.Dead
			e.logger.Error("split order reached dead after retries",
				zap.String("order_no", req.OrderNo), zap.String("last_error", lastErr))
			// 告警：死单需人工介入（差错中心复位重开或管理端核销）
			e.alert(ctx, "【分账死单】自动重试耗尽",
				fmt.Sprintf("订单号：%s\n商户：%d\n最近错误：%s\n请前往差错中心处理", req.OrderNo, req.MerchantID, lastErr))
		}
	}

	if err := e.orderStatusRepo.UpdateResult(ctx, req.OrderNo, successCount, string(status), attempt, nextRetryAt, lastErr); err != nil {
		return err
	}
	e.syncOrderSplitStatus(ctx, req.OrderNo, string(status))
	prom.SplitOrderTotal.WithLabelValues(string(status)).Inc()
	if status == state.Success {
		prom.SplitSuccessRate.Set(1)
	} else {
		prom.SplitSuccessRate.Set(0)
	}
	// 终态发布事件（异步投递，失败不影响主流程）
	if status.IsTerminal() {
		e.publishSplitExecutedEvent(ctx, req, string(status), successCount, receiverCount, lastErr)
	}
	if lastErr != "" {
		e.logger.Warn("split order finalized with error",
			zap.String("order_no", req.OrderNo), zap.String("status", string(status)), zap.Int("success", successCount))
	}
	// 非成功终态需向上层返回错误，避免服务层误报成功（部分成功也返回，交由补偿调度续跑）
	if status != state.Success {
		return fmt.Errorf("split order %s: %s", status, lastErr)
	}
	return nil
}

// syncOrderSplitStatus 分账定态后同步回写 t_order.split_status（仅终态）：
// SUCCESS -> SUCCESS；FAILED/DEAD/SUSPENDED -> FAILED；PARTIAL 不写（交由补偿续跑）。
func (e *Executor) syncOrderSplitStatus(ctx context.Context, orderNo, status string) {
	var orderSplit string
	switch string(status) {
	case "SUCCESS":
		orderSplit = "SUCCESS"
	case "FAILED", "DEAD", "SUSPENDED":
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

// publishSplitExecutedEvent 发布分账执行完成事件到 outbox。
func (e *Executor) publishSplitExecutedEvent(ctx context.Context, req *ExecuteRequest, status string, successCount, receiverCount int, lastErr string) {
	if e.outboxRepo == nil {
		return
	}
	payload := event.SplitOrderExecutedPayload{
		OrderNo:       req.OrderNo,
		MerchantID:    req.MerchantID,
		Status:        status,
		ReceiverCount: receiverCount,
		SuccessCount:  successCount,
		TotalAmount:   req.AllocationsTotal(),
		Degraded:      0,
		LastError:     lastErr,
	}
	if err := e.outboxRepo.PublishEvent(ctx, event.AggregateSplitOrder, req.OrderNo, event.TypeSplitOrderExecuted, payload); err != nil {
		e.logger.Warn("publish split executed event fail",
			zap.String("order_no", req.OrderNo), zap.Error(err))
	}
}
