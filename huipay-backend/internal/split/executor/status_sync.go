package executor

import (
	"context"
	"time"

	"go.uber.org/zap"

	splitrepo "github.com/huipay/huipay-backend/internal/split/repository"
	"github.com/huipay/huipay-backend/internal/split/state"
)

func (e *Executor) recordExecutionStatus(ctx context.Context, orderNo string, a Allocation, status state.Status, attempt int, nextRetryAt time.Time, lastErr string) error {
	model := &splitrepo.SplitOrderStatusModel{
		OrderNo:          orderNo,
		StoreID:          a.StoreID,
		ReceiverEntityID: a.EntityID,
		ReceiverType:     a.EntityType,
		Amount:           a.Amount,
		Level:            a.Level,
		Status:           string(status),
		Attempt:          attempt,
		NextRetryAt:      &nextRetryAt,
		LastError:        lastErr,
	}
	return e.orderRepo.Upsert(ctx, model)
}

func (e *Executor) upsertOrderStatus(ctx context.Context, req *ExecuteRequest, execs []SplitExecutionModel) {
	for _, ex := range execs {
		model := &splitrepo.SplitOrderStatusModel{
			OrderNo:          ex.OrderNo,
			ReceiverEntityID: ex.ReceiverEntityID,
			ReceiverType:     ex.ReceiverType,
			Amount:           ex.Amount,
			Level:            ex.Level,
			Status:           ex.Status,
			ChannelSplitNo:   ex.ChannelSplitNo,
			LastError:        ex.LastError,
			Attempt:          ex.Attempt,
		}
		_ = e.orderRepo.Upsert(ctx, model)
	}
}

func (e *Executor) finalizeOrderStatus(ctx context.Context, orderNo string, successCount int, status state.Status, attempt int, nextRetryAt time.Time, lastErr string) {
	_ = e.orderRepo.UpdateResult(ctx, orderNo, successCount, string(status), attempt, nextRetryAt, lastErr)
}

func (e *Executor) syncOrderSplitStatus(ctx context.Context, orderNo string, status state.Status) {
	switch status {
	case state.Success:
		_ = e.orderRepo.UpdateOrderSplitStatus(ctx, orderNo, "SUCCESS")
	case state.Partial:
		_ = e.orderRepo.UpdateOrderSplitStatus(ctx, orderNo, "PARTIAL")
	case state.Failed:
		_ = e.orderRepo.UpdateOrderSplitStatus(ctx, orderNo, "FAILED")
	}
}

func (e *Executor) countSuccess(execs []SplitExecutionModel) int {
	count := 0
	for _, ex := range execs {
		if ex.Status == string(state.Success) {
			count++
		}
	}
	return count
}
