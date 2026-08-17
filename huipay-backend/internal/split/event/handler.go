package event

import (
	"context"
	"encoding/json"
	"fmt"

	"go.uber.org/zap"

	"github.com/huipay/huipay-backend/infra/notify"
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
// 终态为 DEAD 时触发告警通知。
func SplitOrderExecutedHandler(logger *zap.Logger, alerter notify.Alerter) Handler {
	return func(ctx context.Context, event *DomainEvent) error {
		var payload SplitOrderExecutedPayload
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			logger.Warn("unmarshal SplitOrderExecutedPayload fail", zap.Error(err))
			return nil
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
		// DEAD 终态：触发告警通知运营介入
		if payload.Status == "DEAD" {
			alerter.Alert(ctx, "【分账死单】已达最大重试次数",
				fmt.Sprintf("商户 %d 订单 %s 分账失败已达重试上限，待人工介入。"+
					"金额 %d 分，成功 %d/%d 接收方",
					payload.MerchantID, payload.OrderNo, payload.TotalAmount,
					payload.SuccessCount, payload.ReceiverCount))
		}
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

// ReconcileDiffResolvedHandler 对账差异核销处理器。
func ReconcileDiffResolvedHandler(logger *zap.Logger) Handler {
	return func(ctx context.Context, event *DomainEvent) error {
		var payload ReconcileDiffResolvedPayload
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			logger.Warn("unmarshal ReconcileDiffResolvedPayload fail", zap.Error(err))
			return nil
		}
		logger.Info("reconcile diff resolved",
			zap.Uint64("diff_id", payload.DiffID),
			zap.Uint64("merchant_id", payload.MerchantID),
			zap.String("diff_type", payload.DiffType),
		)
		return nil
	}
}