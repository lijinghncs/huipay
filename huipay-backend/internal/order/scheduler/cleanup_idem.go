// 幂等键过期清理：定期删除 t_idempotency_key 中已过期的记录，避免表无限增长。
package scheduler

import (
	"context"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/huipay/huipay-backend/infra/idem"
)

// StartIdempotencyCleanup 每小时清理过期幂等记录，阻塞运行直到 ctx 取消。
func StartIdempotencyCleanup(ctx context.Context, db *gorm.DB, interval time.Duration, logger *zap.Logger) {
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
	defer func() {
		if r := recover(); r != nil {
			logger.Error("idempotency cleanup panic", zap.Any("panic", r))
		}
	}()
	res := db.WithContext(ctx).
		Where("expire_at < NOW()").
		Limit(1000).
		Delete(&idem.Record{})
	if res.Error != nil {
		logger.Error("idempotency cleanup fail", zap.Error(res.Error))
		return
	}
	if res.RowsAffected > 0 {
		logger.Info("idempotency cleanup",
			zap.Int64("deleted", res.RowsAffected))
	}
}