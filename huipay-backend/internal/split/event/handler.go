package event

import (
	"context"
	"encoding/json"

	"go.uber.org/zap"
)

// LogHandler 事件日志记录器（订阅所有事件，打印到日志）。
func LogHandler(logger *zap.Logger) Handler {
	return func(ctx context.Context, event *DomainEvent) error {
		logger.Info("domain event",
			zap.String("event_id", event.ID),
			zap.String("event_type", event.EventType),
			zap.String("aggregate_type", event.AggregateType),
			zap.String("aggregate_id", event.AggregateID),
			zap.String("payload", string(event.Payload)),
		)
		return nil
	}
}

// SplitOrderExecutedHandler 分账执行完成处理器。
// 当前仅日志记录；后续可扩展为：推送通知 / 触发对账 / 数据同步。
func SplitOrderExecutedHandler(logger *zap.Logger) Handler {
	return func(ctx context.Context, event *DomainEvent) error {
		var payload SplitOrderExecutedPayload
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			logger.Warn("unmarshal SplitOrderExecutedPayload fail", zap.Error(err))
			return nil // 不阻断总线
		}
		logger.Info("split order executed",
			zap.String("order_no", payload.OrderNo),
			zap.Uint64("merchant_id", payload.MerchantID),
			zap.String("status", payload.Status),
			zap.Int("success", payload.SuccessCount),
			zap.Int("total", payload.ReceiverCount),
			zap.Int64("amount", payload.TotalAmount),
			zap.Int("degraded", payload.Degraded),
		)
		return nil
	}
}

// SplitBillApprovedHandler 账单审批通过处理器。
func SplitBillApprovedHandler(logger *zap.Logger) Handler {
	return func(ctx context.Context, event *DomainEvent) error {
		var payload SplitBillApprovedPayload
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			logger.Warn("unmarshal SplitBillApprovedPayload fail", zap.Error(err))
			return nil
		}
		logger.Info("split bill approved",
			zap.String("batch_no", payload.BatchNo),
			zap.Uint64("merchant_id", payload.MerchantID),
			zap.Int64("total_amount", payload.TotalAmount),
		)
		return nil
	}
}

// SplitBillRejectedHandler 账单驳回处理器。
func SplitBillRejectedHandler(logger *zap.Logger) Handler {
	return func(ctx context.Context, event *DomainEvent) error {
		var payload SplitBillRejectedPayload
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			logger.Warn("unmarshal SplitBillRejectedPayload fail", zap.Error(err))
			return nil
		}
		logger.Info("split bill rejected",
			zap.String("batch_no", payload.BatchNo),
			zap.Uint64("merchant_id", payload.MerchantID),
		)
		return nil
	}
}