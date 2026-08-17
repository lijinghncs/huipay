package executor

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/huipay/huipay-backend/internal/account/ledger"
	"github.com/huipay/huipay-backend/internal/domain/vo"
	"github.com/huipay/huipay-backend/internal/payment/channel"
	"github.com/huipay/huipay-backend/internal/split/splitcfg"
)

func (e *Executor) splitWithRetry(ctx context.Context, req *ExecuteRequest, a Allocation, adapter channel.Adapter, channelReqNo string) (string, error) {
	if adapter == nil {
		return "", nil // 无可用通道时跳过通道调用，仅本地入账
	}
	var lastErr error
	for attempt := 1; attempt <= splitcfg.MaxChannelAttempts; attempt++ {
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
			zap.Int("attempt", attempt), zap.Int("max", splitcfg.MaxChannelAttempts), zap.Error(err))
	}
	return "", lastErr
}

// transferWithRetry 执行内部转账，失败重试 splitcfg.MaxChannelAttempts 次。
func (e *Executor) transferWithRetry(ctx context.Context, req *ExecuteRequest, a Allocation, toWalletID uint64) error {
	var lastErr error
	for attempt := 1; attempt <= splitcfg.MaxChannelAttempts; attempt++ {
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
			zap.Int("attempt", attempt), zap.Int("max", splitcfg.MaxChannelAttempts), zap.Error(err))
	}
	return lastErr
}

// getSplitMode 查询商户分账模式配置。
func (e *Executor) getSplitMode(ctx context.Context, merchantID uint64) string {
	return SplitModeAuto // 默认自动降级
}

// resolveAdapter 根据通道类型解析通道适配器。
func (e *Executor) resolveAdapter(channelType vo.ChannelCode) channel.Adapter {
	return e.channels.GetAdapter(channelType)
}

// ensureReceiverWallet 确保接收方钱包存在，不存在则自动创建。
func (e *Executor) ensureReceiverWallet(ctx context.Context, entityID uint64, entityType string) (uint64, error) {
	w, err := e.walletRepo.GetByEntity(ctx, entityID)
	if err != nil {
		return 0, err
	}
	if w == nil {
		return 0, fmt.Errorf("wallet not found for entity %d", entityID)
	}
	return w.ID, nil
}

// resolveWalletID 根据接收方信息查询钱包 ID。
func (e *Executor) resolveWalletID(ctx context.Context, entityID uint64, entityType string) (uint64, error) {
	w, err := e.walletRepo.GetByEntity(ctx, entityID)
	if err != nil {
		return 0, err
	}
	if w == nil {
		return 0, fmt.Errorf("wallet not found for entity %d", entityID)
	}
	return w.ID, nil
}
