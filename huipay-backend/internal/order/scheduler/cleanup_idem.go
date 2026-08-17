// 幂等键过期清理：定期删除 t_idempotency_key 中已过期的记录，避免表无限增长。
package scheduler

import (
	"context"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/huipay/huipay-backend/infra/idem"
	"github.com/huipay/huipay-backend/internal/scheduler/framework"
)

// StartIdempotencyCleanup 每小时清理过期幂等记录，阻塞运行直到 ctx 取消。
func StartIdempotencyCleanup(ctx context.Context, db *gorm.DB, interval time.Duration, logger *zap.Logger) {
	// 注册到监测注册表（保留自有 ticker，仅登记元信息）
	_ = framework.Register(db, logger, framework.TaskConfig{
		Name:        "idem_cleanup",
		DisplayName: "幂等键清理",
		Description: "清理过期幂等记录",
		IntervalSec: int(interval.Seconds()),
		Enabled:     true,
	})
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cleanupOnce(ctx, db, logger)
		}
	}
}

func cleanupOnce(ctx context.Context, db *gorm.DB, logger *zap.Logger) {
	// 接入监测：name=idem_cleanup，每轮写运行日志
	_, _ = framework.RunLogged(ctx, db, framework.GlobalInstanceID(), "idem_cleanup", nil, func() (int64, error) {
		res := db.WithContext(ctx).
			Where("expire_at < NOW()").
			Limit(1000).
			Delete(&idem.Record{})
		if res.Error != nil {
			logger.Error("idempotency cleanup fail", zap.Error(res.Error))
			return 0, res.Error
		}
		if res.RowsAffected > 0 {
			logger.Info("idempotency cleanup", zap.Int64("deleted", res.RowsAffected))
		}
		return res.RowsAffected, nil
	})
}