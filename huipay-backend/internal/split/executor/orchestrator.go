package executor

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"go.uber.org/zap"

	"github.com/huipay/huipay-backend/infra/prom"
)

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
		return e.finalizeOrderStatus(ctx, req, "", "")
	}

	pendingSum, err := e.sumAmounts(pending)
	if err != nil {
		return err
	}

	// 余额预校验
	if err := e.checkBalance(ctx, req.SourceWallet, pendingSum); err != nil {
		e.incFailure("insufficient_balance")
		return e.finalizeOrderStatus(ctx, req, "", err.Error())
	}

	// 分账模式闸门
	mode := e.getSplitMode(ctx, req.MerchantID)
	adapter := e.resolveAdapter(req.Channel)
	degraded := 0
	switch mode {
	case SplitModeLocalOnly:
		adapter = nil
		degraded = 1
	case SplitModeChannelRequired:
		if adapter == nil {
			e.incFailure("channel_fail")
			return e.finalizeOrderStatus(ctx, req, "", "channel required but not configured")
		}
	default: // AUTO
		if adapter == nil {
			degraded = 1
		}
	}

	// 写入订单级 PROCESSING 状态
	if err := e.upsertOrderStatus(ctx, req, pendingSum, degraded); err != nil {
		return err
	}

	// 并行处理各接收方（errs 收集首个错误，atomic 计数成功）
	var (
		successCount int64
		firstErr     atomic.Value
		firstErrMsg  string
		mu           sync.Mutex
		wg           sync.WaitGroup
		sem          = make(chan struct{}, 8) // 并发上限 8
		errCapture   sync.Once
	)
	for _, a := range pending {
		wg.Add(1)
		a := a
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			// 已有错误时跳过剩余接收方
			if firstErr.Load() != nil {
				return
			}

			toWalletID, err := e.ensureReceiverWallet(ctx, a.EntityID, string(a.EntityType))
			if err != nil {
				msg := fmt.Sprintf("wallet for entity=%d: %v", a.EntityID, err)
				firstErr.Store(&msg)
				return
			}

			channelReqNo := channelReqNo(req.OrderNo, a.EntityID)

			channelSplitNo, err := e.splitWithRetry(ctx, req, a, adapter, channelReqNo)
			if err != nil {
				_ = e.recordExecutionStatus(ctx, req, a, "FAILED", err.Error(), "", maxAttempts, degraded, channelReqNo)
				e.incFailure("channel_fail")
				msg := fmt.Sprintf("channel split entity=%d: %v", a.EntityID, err)
				firstErr.Store(&msg)
				return
			}

			if err := e.transferWithRetry(ctx, req, a, toWalletID); err != nil {
				_ = e.recordExecutionStatus(ctx, req, a, "FAILED", err.Error(), channelSplitNo, maxAttempts, degraded, channelReqNo)
				e.incFailure("transfer_fail")
				msg := fmt.Sprintf("transfer to entity=%d: %v", a.EntityID, err)
				firstErr.Store(&msg)
				return
			}

			if err := e.recordExecutionStatus(ctx, req, a, "SUCCESS", "", channelSplitNo, 0, degraded, channelReqNo); err != nil {
				e.logger.Warn("record split execution fail",
					zap.String("order_no", req.OrderNo), zap.Error(err))
			}
			atomic.AddInt64(&successCount, 1)
			prom.SplitAmountTotal.Add(float64(a.Amount))
			e.logger.Info("split transferred",
				zap.String("order_no", req.OrderNo),
				zap.Uint64("to_entity", a.EntityID),
				zap.Int64("amount", a.Amount),
				zap.Int("level", a.Level),
				zap.String("channel_split_no", channelSplitNo),
				zap.String("channel_req_no", channelReqNo),
			)
		}()
	}
	wg.Wait()

	if v := firstErr.Load(); v != nil {
		msg := *v.(*string)
		mu.Lock()
		firstErrMsg = msg
		mu.Unlock()
		return e.finalizeOrderStatus(ctx, req, "", firstErrMsg)
	}
	_ = errCapture // suppress unused

	return e.finalizeOrderStatus(ctx, req, "", "")
}