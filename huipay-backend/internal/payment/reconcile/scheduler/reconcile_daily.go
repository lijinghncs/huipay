// 包 scheduler 提供进程内对账定时任务。
// 每日 09:00 触发，对账前一日（T+1）微信交易账单，差异落库。
package scheduler

import (
	"context"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/huipay/huipay-backend/internal/payment/reconcile"
)

// StartDailyReconcile 启动每日对账调度器，阻塞运行直到 ctx 取消。
// 采用 1 分钟 ticker，命中每日 09:00 所在的分钟窗口执行一次。
func StartDailyReconcile(ctx context.Context, d *reconcile.Downloader, db *gorm.DB, logger *zap.Logger) {
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

func runOnce(ctx context.Context, d *reconcile.Downloader, db *gorm.DB, bizDate string, logger *zap.Logger) {
	defer func() {
		if r := recover(); r != nil {
			logger.Error("reconcile scheduler panic", zap.Any("panic", r))
		}
	}()

	report, err := reconcile.Reconcile(ctx, d, db, bizDate)
	if err != nil {
		logger.Error("reconcile run fail",
			zap.String("biz_date", bizDate), zap.Error(err))
		return
	}
	if err := reconcile.SaveDiffs(ctx, db, report); err != nil {
		logger.Error("reconcile save diff fail",
			zap.String("biz_date", bizDate), zap.Error(err))
		return
	}
	reconcile.LogSummary(logger, report)
}