// Package scheduler 将对账任务注册到监测框架：只负责触发（自动窗口 + 手动触发），
// 业务编排全部在 recon/job 内。
package scheduler

import (
	"context"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/huipay/huipay-backend/internal/recon/engine"
	"github.com/huipay/huipay-backend/internal/scheduler/framework"
)

// NewPostSplitHandle 注册「分账执行后日对账」任务（自动 + 手动触发），返回调度句柄。
func NewPostSplitHandle(db *gorm.DB, job engine.ScheduledJob, logger *zap.Logger) *framework.Handle {
	framework.RegisterManual(job.Name(), manualRun(job))
	return framework.Register(db, logger, framework.TaskConfig{
		Name:        job.Name(),
		DisplayName: "分账日对账",
		Description: "T+1 02:30 对账分账执行记录与账本分录，差异落 t_reconcile_diff",
		CronExpr:    "每日 02:30",
		IntervalSec: 60,
		Enabled:     true,
	})
}

// PostSplitRunnable 自动调度执行体。
func PostSplitRunnable(job engine.ScheduledJob) framework.Runner { return runnable(job) }

// PostSplitOptions 02:30 窗口，业务日期 T-1。
func PostSplitOptions() framework.RunOptions {
	return options(func(now time.Time) bool { return now.Hour() == 2 && now.Minute() == 30 })
}

// NewChannelHandle 注册「渠道日对账」任务（自动 + 手动触发），返回调度句柄。
func NewChannelHandle(db *gorm.DB, job engine.ScheduledJob, logger *zap.Logger) *framework.Handle {
	framework.RegisterManual(job.Name(), manualRun(job))
	return framework.Register(db, logger, framework.TaskConfig{
		Name:        job.Name(),
		DisplayName: "渠道日对账",
		Description: "T+1 09:00 对账本地已支付订单与微信渠道账单，差异落 t_reconcile_diff",
		CronExpr:    "每日 09:00",
		IntervalSec: 60,
		Enabled:     true,
	})
}

// ChannelRunnable 自动调度执行体。
func ChannelRunnable(job engine.ScheduledJob) framework.Runner { return runnable(job) }

// ChannelOptions 09:00 窗口，业务日期 T-1。
func ChannelOptions() framework.RunOptions {
	return options(func(now time.Time) bool { return now.Hour() == 9 && now.Minute() == 0 })
}

func runnable(job engine.ScheduledJob) framework.Runner {
	return func(ctx context.Context, bizDate time.Time) (int64, error) {
		if bizDate.IsZero() {
			return 0, nil
		}
		return job.Run(ctx, bizDate)
	}
}

func manualRun(job engine.ScheduledJob) func(context.Context, time.Time) (int64, error) {
	return func(ctx context.Context, bizDate time.Time) (int64, error) {
		if bizDate.IsZero() {
			bizDate = time.Now().AddDate(0, 0, -1)
		}
		return job.Run(ctx, bizDate)
	}
}

func options(shouldRun func(now time.Time) bool) framework.RunOptions {
	return framework.RunOptions{
		BizDate:   func(now time.Time) time.Time { return now.AddDate(0, 0, -1) },
		ShouldRun: shouldRun,
	}
}
