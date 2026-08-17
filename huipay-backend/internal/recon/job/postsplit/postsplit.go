// Package postsplit 实现执行后对账（T+1，D1 层）：
// 本地账本 SPLIT CREDIT 入账合计 vs 分账执行记录 SUCCESS 合计（按订单逐笔比对）。
package postsplit

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/huipay/huipay-backend/infra/notify"
	"github.com/huipay/huipay-backend/internal/recon/domain"
	"github.com/huipay/huipay-backend/internal/recon/ports"
)

// TaskName 调度任务名（对账中心运行日志与手动触发均使用）。
const TaskName = "split_daily_reconcile"

// Job 执行后对账任务。
type Job struct {
	journal ports.JournalSideFetcher
	execs   ports.ExecutionSideFetcher
	diffs   ports.DiffStore
	alerter notify.Alerter
	logger  *zap.Logger
}

func NewJob(journal ports.JournalSideFetcher, execs ports.ExecutionSideFetcher, diffs ports.DiffStore, alerter notify.Alerter, logger *zap.Logger) *Job {
	if alerter == nil {
		alerter = notify.NoopAlerter{}
	}
	return &Job{journal: journal, execs: execs, diffs: diffs, alerter: alerter, logger: logger}
}

func (j *Job) Name() string { return TaskName }

// Run 对账指定业务日（[bizDate, bizDate+1)）：逐订单比对执行记录与本地账本，差异落库。返回差异条数。
func (j *Job) Run(ctx context.Context, bizDate time.Time) (int64, error) {
	start := time.Date(bizDate.Year(), bizDate.Month(), bizDate.Day(), 0, 0, 0, 0, time.Local)
	end := start.AddDate(0, 0, 1)

	merchantIDs, err := j.execs.ListMerchantsWithExecution(ctx, start, end)
	if err != nil {
		return 0, fmt.Errorf("list merchants with execution: %w", err)
	}
	if len(merchantIDs) == 0 {
		return 0, nil
	}

	var totalDiffs int64
	for _, mid := range merchantIDs {
		n, err := j.reconcileMerchant(ctx, mid, start, end)
		if err != nil {
			j.logger.Warn("split daily reconcile merchant fail", zap.Uint64("merchant", mid), zap.Error(err))
			continue
		}
		totalDiffs += n
	}
	return totalDiffs, nil
}

// reconcileMerchant 对账单商户：逐订单比对 + 降级订单强制入差异；幂等重写（未核销部分）。返回差异条数。
func (j *Job) reconcileMerchant(ctx context.Context, merchantID uint64, start, end time.Time) (int64, error) {
	execRows, err := j.execs.SumByOrder(ctx, merchantID, start, end)
	if err != nil {
		return 0, fmt.Errorf("sum execution by order: %w", err)
	}
	if len(execRows) == 0 {
		return 0, nil
	}

	orderNos := make([]string, 0, len(execRows))
	for _, r := range execRows {
		orderNos = append(orderNos, r.OrderNo)
	}
	journalByOrder, err := j.journal.SumByOrderNos(ctx, orderNos)
	if err != nil {
		return 0, fmt.Errorf("sum journal by order: %w", err)
	}

	// 逐订单比对：金额不平 → SPLIT_POST；降级订单 → SPLIT_DEGRADED（提示级，供人工核销）
	var postDiffs, degradedDiffs []domain.Diff
	for _, r := range execRows {
		journal := journalByOrder[r.OrderNo]
		if r.Degraded == 1 {
			degradedDiffs = append(degradedDiffs, domain.Diff{
				OrderNo:      r.OrderNo,
				LocalAmount:  domain.Int64(r.ExecSum),
				RemoteAmount: domain.Int64(0),
				Reason:       domain.ReasonDegraded,
				Detail:       fmt.Sprintf(`{"reason":"本地入账、通道未分","exec_sum":%d}`, r.ExecSum),
			})
			continue
		}
		if journal != r.ExecSum {
			postDiffs = append(postDiffs, domain.Diff{
				OrderNo:      r.OrderNo,
				LocalAmount:  domain.Int64(journal),
				RemoteAmount: domain.Int64(r.ExecSum),
				Reason:       domain.ReasonJournalVsExec,
				Detail:       fmt.Sprintf(`{"journal_sum":%d,"exec_sum":%d}`, journal, r.ExecSum),
			})
		}
	}

	// 幂等落库（保留已核销）
	bizDate := start
	mid := merchantID
	written := int64(0)
	if n, err := j.diffs.WriteOrderDiffs(ctx, &mid, bizDate, domain.DiffTypeSplitPost, postDiffs); err != nil {
		return 0, fmt.Errorf("write post diffs: %w", err)
	} else {
		written += int64(n)
	}
	if n, err := j.diffs.WriteOrderDiffs(ctx, &mid, bizDate, domain.DiffTypeSplitDegraded, degradedDiffs); err != nil {
		return 0, fmt.Errorf("write degraded diffs: %w", err)
	} else {
		written += int64(n)
	}

	// 差异告警（金额不平才告警；降级订单量大，仅记日志）
	if len(postDiffs) > 0 {
		j.alerter.Alert(ctx, "【分账对账差异】执行后与账本不平",
			fmt.Sprintf("商户：%d\n业务日：%s\n不平订单数：%d\n请前往差错中心核对处理",
				merchantID, bizDate.Format("2006-01-02"), len(postDiffs)))
	}
	if len(degradedDiffs) > 0 {
		j.logger.Info("split degraded orders recorded",
			zap.Uint64("merchant", merchantID), zap.Int("count", len(degradedDiffs)))
	}
	return written, nil
}
