// 包 scheduler 提供分账补偿调度：悬挂检测 + 失败/部分失败订单自动重入。
package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/huipay/huipay-backend/infra/notify"
	"github.com/huipay/huipay-backend/infra/prom"
	"github.com/huipay/huipay-backend/internal/account/service"
	"github.com/huipay/huipay-backend/internal/domain/vo"
	"github.com/huipay/huipay-backend/internal/scheduler/framework"
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
	alerter         notify.Alerter
	logger          *zap.Logger
}

// NewCompensateScheduler 构造调度器。
func NewCompensateScheduler(osr *repository.SplitOrderStatusRepo, ex *executor.Executor, account *service.Service, logger *zap.Logger) *CompensateScheduler {
	// 注册到监测注册表（保留自有 ticker，仅登记元信息）
	_ = framework.Register(osr.DB(), logger, framework.TaskConfig{
		Name:        "split_compensate",
		DisplayName: "分账补偿",
		Description: "悬挂检测 + 失败/部分失败订单自动重入",
		IntervalSec: 30,
		Enabled:     true,
	})
	return &CompensateScheduler{orderStatusRepo: osr, executor: ex, account: account, logger: logger}
}

// SetAlerter 注入告警器（可选；不注入则告警为空操作）。
func (s *CompensateScheduler) SetAlerter(a notify.Alerter) {
	if a == nil {
		a = notify.NoopAlerter{}
	}
	s.alerter = a
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
				// 接入监测：name=split_compensate，每轮写运行日志
				_, _ = framework.RunLogged(ctx, s.orderStatusRepo.DB(), framework.GlobalInstanceID(), "split_compensate", nil, func() (int64, error) {
					return s.runOnce(ctx)
				})
			}
		}
	}()
}

// runOnce 执行一轮补偿：先悬挂检测，再重入到期的失败/部分失败订单。返回重入成功数。
func (s *CompensateScheduler) runOnce(ctx context.Context) (int64, error) {
	now := time.Now()
	successCount := int64(0)

	// B2 悬挂检测：处理中超时 → SUSPENDED（并入重入范围）
	if n, err := s.orderStatusRepo.MarkSuspended(ctx, now.Add(-hangThreshold)); err != nil {
		s.logger.Warn("split mark suspended fail", zap.Error(err))
	} else if n > 0 {
		s.logger.Info("split hanging detected", zap.Int64("count", n))
		if s.alerter != nil {
			s.alerter.Alert(ctx, "【分账悬挂】处理中超时订单",
				fmt.Sprintf("检测到 %d 笔分账订单处理中超时（>%s），已置悬挂并纳入自动补偿", n, hangThreshold))
		}
	}
	if c, err := s.orderStatusRepo.SuspendedCount(ctx); err == nil {
		prom.SplitHangingTotal.Set(float64(c))
	}

	// B1 补偿重入
	candidates, err := s.orderStatusRepo.ListRetryCandidates(ctx, now, batchSize)
	if err != nil {
		s.logger.Warn("split list retry candidates fail", zap.Error(err))
		return 0, err
	}
	for _, orderNo := range candidates {
		if ok := s.reconcileOne(ctx, orderNo); ok {
			successCount++
		}
	}
	return successCount, nil
}

// reconcileOne 对单个订单补偿重入：按快照重建分配 → executor 重跑（幂等跳过已成功）。
// 返回是否执行了重跑（认领成功且重建分配成功）。
func (s *CompensateScheduler) reconcileOne(ctx context.Context, orderNo string) bool {
	// 原子认领：仅当状态可重试且到期才置 PROCESSING，避免多实例/并发重复补偿
	ok, err := s.orderStatusRepo.ClaimRetry(ctx, orderNo, time.Now())
	if err != nil {
		s.logger.Warn("split claim retry fail", zap.String("order_no", orderNo), zap.Error(err))
		return false
	}
	if !ok {
		return false
	}

	st, err := s.orderStatusRepo.Get(ctx, orderNo)
	if err != nil || st == nil {
		s.logger.Warn("split order status not found", zap.String("order_no", orderNo), zap.Error(err))
		return false
	}

	var allocations []executor.Allocation
	if err := json.Unmarshal([]byte(st.RuleSnapshot), &allocations); err != nil || len(allocations) == 0 {
		s.logger.Warn("split compensate skip: no valid snapshot", zap.String("order_no", orderNo))
		return false
	}

	wallet, wErr := s.account.GetWalletByEntityType(ctx, st.MerchantID, vo.EntityMerchant)
	if wErr != nil || wallet == nil {
		s.logger.Warn("split compensate skip: merchant wallet not found",
			zap.String("order_no", orderNo), zap.Uint64("merchant", st.MerchantID), zap.Error(wErr))
		return false
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
	return true
}

// derefUint 解引用 uint64 指针，nil 返回 0。
func derefUint(p *uint64) uint64 {
	if p == nil {
		return 0
	}
	return *p
}