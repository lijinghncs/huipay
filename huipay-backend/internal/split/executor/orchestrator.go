package executor

import (
	"context"
	"fmt"

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
		// 全部已成功，幂等完成
		return e.finalizeOrderStatus(ctx, req, "", "")
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
		if _, err := e.ensureReceiverWallet(ctx, a.EntityID, string(a.EntityType)); err != nil {
			return e.finalizeOrderStatus(ctx, req, "", fmt.Sprintf("ensure wallet entity=%d: %v", a.EntityID, err))
		}
		toWalletID, err := e.resolveWalletID(ctx, a.EntityID, string(a.EntityType))
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
