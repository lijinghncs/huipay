// 包 scheduler 提供门店订单日报定时任务（T+1 02:00 聚合 T 日订单）。
package scheduler

import (
	"context"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/huipay/huipay-backend/internal/scheduler/framework"
	statsservice "github.com/huipay/huipay-backend/internal/stats/service"
)

// taskName 门店日报任务唯一名。
const taskName = "store_daily_stats"

// NewStoreDailyStatsScheduler 注册并返回门店日报调度句柄。
func NewStoreDailyStatsScheduler(db *gorm.DB, svc *statsservice.Service, logger *zap.Logger) *framework.Handle {
	// 注册手动触发执行体（admin 端强制重跑）
	framework.RegisterManual(taskName, func(ctx context.Context, bizDate time.Time) (int64, error) {
		if bizDate.IsZero() {
			bizDate = time.Now().AddDate(0, 0, -1)
		}
		return svc.GenerateDailyStats(ctx, bizDate)
	})
	return framework.Register(db, logger, framework.TaskConfig{
		Name:        taskName,
		DisplayName: "门店订单日报",
		Description: "T+1 02:00 聚合 T 日订单按门店写入日报",
		CronExpr:    "每天 02:00",
		IntervalSec: 60, // 1 分钟 tick 用于命中 02:00 窗口
		Enabled:     true,
	})
}

// Runnable 返回适配 framework.Runner 的执行体（供 Start 使用）。
func Runnable(svc *statsservice.Service) framework.Runner {
	return func(ctx context.Context, bizDate time.Time) (int64, error) {
		if bizDate.IsZero() {
			return 0, nil
		}
		return svc.GenerateDailyStats(ctx, bizDate)
	}
}

// Options 返回该任务的触发选项：命中 02:00 窗口 + bizDate=昨日。
func Options() framework.RunOptions {
	return framework.RunOptions{
		BizDate: func(now time.Time) time.Time {
			return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local).AddDate(0, 0, -1)
		},
		ShouldRun: func(now time.Time) bool {
			// 命中 02:00 所在分钟窗口（分钟级精度）
			return now.Hour() == 2 && now.Sub(now.Truncate(24*time.Hour)) < time.Minute
		},
	}
}