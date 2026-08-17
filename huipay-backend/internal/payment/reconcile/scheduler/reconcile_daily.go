// 包 scheduler 提供进程内对账定时任务。
// 每日 09:00 触发，对账前一日（T+1）微信交易账单，差异落库。
package scheduler

import (
	"context"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/huipay/huipay-backend/internal/payment/reconcile"
	"github.com/huipay/huipay-backend/internal/scheduler/framework"
)

// StartDailyReconcile 启动每日对账调度器，阻塞运行直到 ctx 取消。
// 采用 1 分钟 ticker，命中每日 09:00 所在的分钟窗口执行一次。
func StartDailyReconcile(ctx context.Context, d *reconcile.Downloader, db *gorm.DB, logger *zap.Logger) {
	// 注册到监测注册表（保留自有 ticker，仅登记元信息）
	_ = framework.Register(db, logger, framework.TaskConfig{
		Name:        taskName,
		DisplayName: "每日对账",
		Description: "T+1 09:00 对账前一日微信交易账单",
		CronExpr:    "每天 09:00",
		IntervalSec: 60,
		Enabled:     true,
	})
	// 注册手动触发执行体（admin 端强制重跑昨日账单）
	framework.RegisterManual(taskName, func(ctx context.Context, bizDate time.Time) (int64, error) {
		if bizDate.IsZero() {
			bizDate = time.Now().AddDate(0, 0, -1)
		}
		return runOnceReturn(ctx, d, db, bizDate.Format("2006-01-02"), logger)
	})

	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	var lastRunDate string
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			// 命中 09:00 所在分钟窗口（分钟级精度，避免跨日 sleep）
			if now.Hour() == 9 && now.Sub(now.Truncate(24*time.Hour)) < time.Minute {
				today := now.Format("2006-01-02")
				if today != lastRunDate {
					lastRunDate = today
					// 对账昨天的账单（避免 09:00 时今日账单尚未生成）
					bizDate := now.AddDate(0, 0, -1).Format("2006-01-02")
					runOnce(ctx, d, db, bizDate, logger)
				}
			}
		}
	}
}

// taskName 对账任务唯一名。
const taskName = "reconcile_daily"

// runOnceReturn 执行对账并返回影响行数（供手动触发复用）。
func runOnceReturn(ctx context.Context, d *reconcile.Downloader, db *gorm.DB, bizDate string, logger *zap.Logger) (int64, error) {
	report, err := reconcile.Reconcile(ctx, d, db, bizDate)
	if err != nil {
		logger.Error("reconcile run fail", zap.String("biz_date", bizDate), zap.Error(err))
		return 0, err
	}
	if err := reconcile.SaveDiffs(ctx, db, report); err != nil {
		logger.Error("reconcile save diff fail", zap.String("biz_date", bizDate), zap.Error(err))
		return 0, err
	}
	reconcile.LogSummary(logger, report)
	return int64(len(report.LongOrders) + len(report.ShortOrders) + len(report.MismatchOrders)), nil
}

func runOnce(ctx context.Context, d *reconcile.Downloader, db *gorm.DB, bizDate string, logger *zap.Logger) {
	bd := parseDate(bizDate)
	// 接入监测：name=reconcile_daily，写运行日志
	_, _ = framework.RunLogged(ctx, db, framework.GlobalInstanceID(), "reconcile_daily", bd, func() (int64, error) {
		return runOnceReturn(ctx, d, db, bizDate, logger)
	})
}

// parseDate 解析 YYYY-MM-DD 为 time.Time；失败返回 nil。
func parseDate(s string) *time.Time {
	t, err := time.ParseInLocation("2006-01-02", s, time.Local)
	if err != nil {
		return nil
	}
	return &t
}