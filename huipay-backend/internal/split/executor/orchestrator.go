package executor

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/huipay/huipay-backend/internal/domain/vo"
	"github.com/huipay/huipay-backend/internal/payment/router"
	splitrepo "github.com/huipay/huipay-backend/internal/split/repository"
	"github.com/huipay/huipay-backend/internal/split/state"
)

func (e *Executor) Execute(ctx context.Context, req *ExecuteRequest) error {
	successCount := 0
	lastErr := ""
	for _, a := range req.Allocations {
		done, err := e.hasSuccess(ctx, req.OrderNo, a.EntityID)
		if err != nil {
			return err
		}
		if done {
			successCount++
			continue
		}
		status, err := e.splitWithRetry(ctx, req, a)
		if err != nil {
			lastErr = err.Error()
			_ = e.recordExecutionStatus(ctx, req.OrderNo, a, state.Failed, 0, time.Time{}, lastErr)
			incFailure("channel_split")
			continue
		}
		_ = e.recordExecutionStatus(ctx, req.OrderNo, a, status, 0, time.Time{}, "")
		if status == state.Success {
			successCount++
		}
	}

	status, attempt := state.Failed, 0
	nextRetryAt := time.Time{}
	if successCount > 0 {
		status = state.Partial
	}
	if successCount == len(req.Allocations) {
		status = state.Success
	}

	e.finalizeOrderStatus(ctx, req.OrderNo, successCount, status, attempt, nextRetryAt, lastErr)
	e.syncOrderSplitStatus(ctx, req.OrderNo, status)
	prom.SplitOrderTotal.WithLabelValues(status).Inc()
	if lastErr != "" {
		return fmt.Errorf("split order %s: %s", status, lastErr)
	}
	return nil
}

func (e *Executor) hasSuccess(ctx context.Context, orderNo string, entityID uint64) (bool, error) {
	models, err := e.orderRepo.ListByOrderNo(ctx, orderNo)
	if err != nil {
		return false, err
	}
	for _, m := range models {
		if m.ReceiverEntityID == entityID && m.Status == string(state.Success) {
			return true, nil
		}
	}
	return false, nil
}

func (e *Executor) splitWithRetry(ctx context.Context, req *ExecuteRequest, a Allocation) (state.Status, error) {
	mode := e.getSplitMode(req.Channel, a)
	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		status, err := e.transferWithRetry(ctx, req, a, mode, attempt)
		if err != nil {
			lastErr = err
			time.Sleep(time.Duration(attempt) * 100 * time.Millisecond)
			continue
		}
		return status, nil
	}
	return state.Failed, lastErr
}

func (e *Executor) transferWithRetry(ctx context.Context, req *ExecuteRequest, a Allocation, mode string, attempt int) (state.Status, error) {
	adapter := e.resolveAdapter(req.Channel, a)
	if adapter == nil {
		return state.Failed, fmt.Errorf("no adapter for channel=%s entity_type=%s", req.Channel, a.EntityType)
	}
	walletID, err := e.resolveWalletID(ctx, a)
	if err != nil {
		return state.Failed, err
	}
	if mode == SplitModeLocalOnly || (mode == SplitModeAuto && adapter == nil) {
		_, err := e.ledgerSvc.Transfer(ctx, ledger.TransferRequest{
			ReceiverID:   walletID,
			Amount:       a.Amount,
			OrderNo:      req.OrderNo,
			MerchantID:   req.MerchantID,
		})
		if err != nil {
			return state.Failed, err
		}
		return state.Success, nil
	}
	chReq := &router.SplitRequest{
		OrderNo:  channelReqNo(req.OrderNo, a.EntityID, attempt),
		Amount:   a.Amount,
		Receiver: router.Receiver{ID: a.EntityID, Type: a.EntityType},
	}
	_, err = adapter.Split(ctx, chReq)
	if err != nil {
		return state.Failed, err
	}
	return state.Success, nil
}

func (e *Executor) resolveAdapter(channelCode string, a Allocation) router.Splitter {
	if e.router == nil {
		return nil
	}
	return e.router.Resolve(vo.ChannelCode(channelCode))
}

func (e *Executor) getSplitMode(channelCode string, a Allocation) string {
	if a.ReceiverScope == "" {
		return SplitModeAuto
	}
	return SplitModeChannelReq
}

func (e *Executor) ensureReceiverWallet(ctx context.Context, a Allocation) error {
	_, err := e.walletRepo.GetByEntity(ctx, a.EntityID)
	return err
}

func (e *Executor) resolveWalletID(ctx context.Context, a Allocation) (uint64, error) {
	wallet, err := e.walletRepo.GetByEntity(ctx, a.EntityID)
	if err != nil {
		return 0, err
	}
	if wallet == nil {
		return 0, fmt.Errorf("wallet not found for entity %d", a.EntityID)
	}
	return wallet.ID, nil
}
