// 包 scheduler 提供分账补偿调度：悬挂检测 + 失败/部分失败订单自动重入。
package scheduler

import (
	"context"
	"encoding/json"
	"time"

	"go.uber.org/zap"

	"github.com/huipay/huipay-backend/infra/prom"
	"github.com/huipay/huipay-backend/internal/account/service"
	"github.com/huipay/huipay-backend/internal/domain/vo"
	"github.com/huipay/huipay-backend/internal/split/executor"
	"github.com/huipay/huipay-backend/internal/split/repository"
)

// hangThreshold 悬挂判定阈值：PROCESSING 超过该时长未更新视为悬挂。
const hangThreshold = 10 * time.Minute

// batchSize 单轮补偿处理上限。
const batchSize = 50

// CompensateScheduler 分账补偿调度器（B1 重入 + B2 悬挂检测）。
type CompensateScheduler struct {
	orderStatusRepo *repository.SplitOrderStatusRepo
	executor        *executor.Executor
	account         *service.Service
	logger          *zap.Logger
}

// NewCompensateScheduler 构造调度器。
func NewCompensateScheduler(osr *repository.SplitOrderStatusRepo, ex *executor.Executor, account *service.Service, logger *zap.Logger) *CompensateScheduler {
	return &CompensateScheduler{orderStatusRepo: osr, executor: ex, account: account, logger: logger}
}

// Start 启动后台补偿循环，interval 为轮询间隔。
func (s *CompensateScheduler) Start(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.runOnce(ctx)
			}
		}
	}()
}

// runOnce 执行一轮补偿：先悬挂检测，再重入到期的失败/部分失败订单。
func (s *CompensateScheduler) runOnce(ctx context.Context) {
	now := time.Now()

	// B2 悬挂检测：处理中超时 → SUSPENDED（并入重入范围）
	if n, err := s.orderStatusRepo.MarkSuspended(ctx, now.Add(-hangThreshold)); err != nil {
		s.logger.Warn("split mark suspended fail", zap.Error(err))
	} else if n > 0 {
		s.logger.Info("split hanging detected", zap.Int64("count", n))
	}
	if c, err := s.orderStatusRepo.SuspendedCount(ctx); err == nil {
		prom.SplitHangingTotal.Set(float64(c))
	}

	// B1 补偿重入
	candidates, err := s.orderStatusRepo.ListRetryCandidates(ctx, now, batchSize)
	if err != nil {
		s.logger.Warn("split list retry candidates fail", zap.Error(err))
		return
	}
	for _, orderNo := range candidates {
		s.reconcileOne(ctx, orderNo)
	}
}

// reconcileOne 对单个订单补偿重入：按快照重建分配 → executor 重跑（幂等跳过已成功）。
func (s *CompensateScheduler) reconcileOne(ctx context.Context, orderNo string) {
	// 原子认领：仅当状态可重试且到期才置 PROCESSING，避免多实例/并发重复补偿
	ok, err := s.orderStatusRepo.ClaimRetry(ctx, orderNo, time.Now())
	if err != nil {
		s.logger.Warn("split claim retry fail", zap.String("order_no", orderNo), zap.Error(err))
		return
	}
	if !ok {
		return
	}

	st, err := s.orderStatusRepo.Get(ctx, orderNo)
	if err != nil || st == nil {
		s.logger.Warn("split order status not found", zap.String("order_no", orderNo), zap.Error(err))
		return
	}

	var allocations []executor.Allocation
	if err := json.Unmarshal([]byte(st.RuleSnapshot), &allocations); err != nil || len(allocations) == 0 {
		s.logger.Warn("split compensate skip: no valid snapshot", zap.String("order_no", orderNo))
		return
	}

	wallet, wErr := s.account.GetWalletByEntityType(ctx, st.MerchantID, vo.EntityMerchant)
	if wErr != nil || wallet == nil {
		s.logger.Warn("split compensate skip: merchant wallet not found",
			zap.String("order_no", orderNo), zap.Uint64("merchant", st.MerchantID), zap.Error(wErr))
		return
	}

	prom.SplitRetryTotal.Inc()
	if err := s.executor.Execute(ctx, &executor.ExecuteRequest{
		MerchantID:   st.MerchantID,
		OrderNo:      orderNo,
		SourceWallet: wallet.ID,
		Allocations:  allocations,
		RuleID:       derefUint(st.RuleID),
	}); err != nil {
		// 失败时 finalizeOrderStatus 已回写失败态与下次重试时间，此处仅记录
		s.logger.Warn("split compensate execute fail", zap.String("order_no", orderNo), zap.Error(err))
	}
}

// derefUint 解引用 uint64 指针，nil 返回 0。
func derefUint(p *uint64) uint64 {
	if p == nil {
		return 0
	}
	return *p
}