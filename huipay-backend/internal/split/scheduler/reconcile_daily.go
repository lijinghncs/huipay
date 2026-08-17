// 包 scheduler 提供分账执行后 T+1 对账：本地账本 SPLIT 入账合计 vs 分账执行记录 SUCCESS 合计（按订单逐笔比对）。
package scheduler

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/huipay/huipay-backend/infra/notify"
	"github.com/huipay/huipay-backend/internal/scheduler/framework"
	"github.com/huipay/huipay-backend/internal/split/repository"
)

// reconcileTaskName 分账日对账任务唯一名。
const reconcileTaskName = "split_daily_reconcile"

// SplitReconcileScheduler 分账执行后日对账（D1 层：journal vs execution；降级订单强制入差异清单）。
type SplitReconcileScheduler struct {
	db       *gorm.DB
	diffRepo *repository.ReconcileDiffRepo
	alerter  notify.Alerter
	logger   *zap.Logger
}

// NewSplitReconcileScheduler 注册并返回调度句柄。
func NewSplitReconcileScheduler(db *gorm.DB, diffRepo *repository.ReconcileDiffRepo, alerter notify.Alerter, logger *zap.Logger) *framework.Handle {
	s := &SplitReconcileScheduler{db: db, diffRepo: diffRepo, alerter: alerter, logger: logger}
	if s.alerter == nil {
		s.alerter = notify.NoopAlerter{}
	}
	// 注册手动触发执行体（商户端/管理端手动重跑，bizDate 缺省昨日）
	framework.RegisterManual(reconcileTaskName, func(ctx context.Context, bizDate time.Time) (int64, error) {
		if bizDate.IsZero() {
			bizDate = time.Now().AddDate(0, 0, -1)
		}
		return s.Reconcile(ctx, bizDate)
	})
	return framework.Register(db, logger, framework.TaskConfig{
		Name:        reconcileTaskName,
		DisplayName: "分账日对账",
		Description: "T+1 02:30 比对本地账本 SPLIT 入账与分账执行记录，差异落 t_reconcile_diff",
		CronExpr:    "每天 02:30",
		IntervalSec: 60, // 1 分钟 tick 用于命中 02:30 窗口
		Enabled:     true,
	})
}

// ReconcileRunnable 返回适配 framework.Runner 的执行体。
func ReconcileRunnable(db *gorm.DB, diffRepo *repository.ReconcileDiffRepo, alerter notify.Alerter, logger *zap.Logger) framework.Runner {
	s := &SplitReconcileScheduler{db: db, diffRepo: diffRepo, alerter: alerter, logger: logger}
	if s.alerter == nil {
		s.alerter = notify.NoopAlerter{}
	}
	return func(ctx context.Context, bizDate time.Time) (int64, error) {
		if bizDate.IsZero() {
			return 0, nil
		}
		return s.Reconcile(ctx, bizDate)
	}
}

// ReconcileOptions 返回触发选项：命中 02:30 窗口 + bizDate=昨日。
func ReconcileOptions() framework.RunOptions {
	return framework.RunOptions{
		BizDate: func(now time.Time) time.Time {
			return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local).AddDate(0, 0, -1)
		},
		ShouldRun: func(now time.Time) bool {
			return now.Hour() == 2 && now.Minute() == 30
		},
	}
}

// orderExecSum 单订单执行侧聚合结果。
type orderExecSum struct {
	OrderNo  string
	ExecSum  int64
	Degraded int
}

// Reconcile 对账指定业务日（[bizDate, bizDate+1)）：逐订单比对执行记录与本地账本，差异落库。返回差异条数。
func (s *SplitReconcileScheduler) Reconcile(ctx context.Context, bizDate time.Time) (int64, error) {
	start := time.Date(bizDate.Year(), bizDate.Month(), bizDate.Day(), 0, 0, 0, 0, time.Local)
	end := start.AddDate(0, 0, 1)

	// 1) 找出当日有成功分账执行的商户集合（经订单级状态表归属，merchant_id 非空且有索引）
	var merchantIDs []uint64
	if err := s.db.WithContext(ctx).Raw(
		`SELECT DISTINCT os.merchant_id
		FROM t_split_execution se
		JOIN t_split_order_status os ON os.order_no = se.order_no
		WHERE se.executed_at >= ? AND se.executed_at < ? AND se.status = 'SUCCESS'`,
		start, end,
	).Scan(&merchantIDs).Error; err != nil {
		return 0, err
	}
	if len(merchantIDs) == 0 {
		return 0, nil
	}

	var totalDiffs int64
	for _, mid := range merchantIDs {
		n, err := s.reconcileMerchant(ctx, mid, start, end)
		if err != nil {
			s.logger.Warn("split daily reconcile merchant fail", zap.Uint64("merchant", mid), zap.Error(err))
			continue
		}
		totalDiffs += n
	}
	return totalDiffs, nil
}

// reconcileMerchant 对账单商户：逐订单比对 + 降级订单强制入差异；幂等重写（未核销部分）。返回差异条数。
func (s *SplitReconcileScheduler) reconcileMerchant(ctx context.Context, merchantID uint64, start, end time.Time) (int64, error) {
	// 2) 执行侧：按订单聚合 SUCCESS 金额与降级标记（经订单级状态表归属商户）
	var execRows []orderExecSum
	if err := s.db.WithContext(ctx).Raw(
		`SELECT se.order_no, SUM(se.amount) AS exec_sum, MAX(se.degraded) AS degraded
		FROM t_split_execution se
		JOIN t_split_order_status os ON os.order_no = se.order_no
		WHERE se.executed_at >= ? AND se.executed_at < ? AND se.status = 'SUCCESS'
			AND os.merchant_id = ?
		GROUP BY se.order_no`,
		start, end, merchantID,
	).Scan(&execRows).Error; err != nil {
		return 0, err
	}
	if len(execRows) == 0 {
		return 0, nil
	}

	// 3) 账本侧：SPLIT CREDIT 流水按订单聚合
	orderNos := make([]string, 0, len(execRows))
	for _, r := range execRows {
		orderNos = append(orderNos, r.OrderNo)
	}
	type journalSum struct {
		BizID string
		Sum   int64
	}
	var journalRows []journalSum
	if err := s.db.WithContext(ctx).Raw(
		`SELECT biz_id, SUM(amount) AS sum FROM t_journal_entry
		WHERE biz_type = 'SPLIT' AND direction = 'CREDIT' AND biz_id IN ?
		GROUP BY biz_id`,
		orderNos,
	).Scan(&journalRows).Error; err != nil {
		return 0, err
	}
	journalByOrder := make(map[string]int64, len(journalRows))
	for _, r := range journalRows {
		journalByOrder[r.BizID] = r.Sum
	}

	// 4) 逐订单比对：金额不平 → SPLIT_POST；降级订单 → SPLIT_DEGRADED（提示级，供人工核销）
	var postDiffs, degradedDiffs []repository.ReconcileDiffModel
	for _, r := range execRows {
		journal := journalByOrder[r.OrderNo]
		if r.Degraded == 1 {
			degradedDiffs = append(degradedDiffs, repository.ReconcileDiffModel{
				OrderNo:       &r.OrderNo,
				LocalAmount:   &r.ExecSum,
				ChannelAmount: ptrInt64(0),
				Detail:        fmt.Sprintf(`{"reason":"本地入账、通道未分","exec_sum":%d}`, r.ExecSum),
			})
			continue
		}
		if journal != r.ExecSum {
			postDiffs = append(postDiffs, repository.ReconcileDiffModel{
				OrderNo:       &r.OrderNo,
				LocalAmount:   &journal,
				ChannelAmount: &r.ExecSum,
				Detail:        fmt.Sprintf(`{"journal_sum":%d,"exec_sum":%d}`, journal, r.ExecSum),
			})
		}
	}

	// 5) 幂等落库（保留已核销）
	bizDate := start
	written := int64(0)
	if n, err := s.diffRepo.WriteSplitPost(ctx, merchantID, bizDate, repository.DiffTypeSplitPost, postDiffs); err != nil {
		return 0, err
	} else {
		written += int64(n)
	}
	if n, err := s.diffRepo.WriteSplitPost(ctx, merchantID, bizDate, repository.DiffTypeSplitDegraded, degradedDiffs); err != nil {
		return 0, err
	} else {
		written += int64(n)
	}

	// 6) 差异告警（金额不平才告警；降级订单量大，仅记日志）
	if len(postDiffs) > 0 {
		s.alerter.Alert(ctx, "【分账对账差异】执行后与账本不平",
			fmt.Sprintf("商户：%d\n业务日：%s\n不平订单数：%d\n请前往差错中心核对处理",
				merchantID, bizDate.Format("2006-01-02"), len(postDiffs)))
	}
	if len(degradedDiffs) > 0 {
		s.logger.Info("split degraded orders recorded",
			zap.Uint64("merchant", merchantID), zap.Int("count", len(degradedDiffs)))
	}
	return written, nil
}

// ptrInt64 取 int64 指针。
func ptrInt64(v int64) *int64 { return &v }
