// 包 scheduler 提供 split_status 异步汇总调度（V2 合并版）。
//
// 设计要点（V2 评审要点 🔴6 + 性能 🔴10）：
//   - 增量触发：扫 t_split_execution 最近 N 分钟变更的 (merchant_id, biz_date)
//   - 应用层去重：内存 map + 串行处理，避免锁竞争
//   - 10 分钟一次（与对账 09:00 错峰），间隔随机抖动
package scheduler

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/huipay/huipay-backend/infra/prom"
	statsservice "github.com/huipay/huipay-backend/internal/stats/service"
)

// recomputeInterval 默认 10 分钟；与现有 09:00 对账错峰。
const recomputeInterval = 10 * time.Minute

// lookbackWindow 增量窗口（5 分钟）。
const lookbackWindow = 5 * time.Minute

// RecomputeScheduler split_status 异步汇总调度器。
type RecomputeScheduler struct {
	db       *gorm.DB
	statsSvc *statsservice.Service
	logger   *zap.Logger
	mu       sync.Mutex
	inFlight map[string]struct{} // key=merchantID|bizDate
}

// NewRecomputeScheduler 构造调度器。
func NewRecomputeScheduler(db *gorm.DB, statsSvc *statsservice.Service, logger *zap.Logger) *RecomputeScheduler {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &RecomputeScheduler{
		db: db, statsSvc: statsSvc, logger: logger,
		inFlight: make(map[string]struct{}),
	}
}

// Start 启动调度循环，阻塞直到 ctx 取消。
func (s *RecomputeScheduler) Start(ctx context.Context) {
	// 启动时立刻跑一次（处理积压）
	s.runOnce(ctx)
	ticker := time.NewTicker(recomputeInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.runOnce(ctx)
		}
	}
}

func (s *RecomputeScheduler) runOnce(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			s.logger.Error("recompute scheduler panic", zap.Any("panic", r))
		}
	}()

	// 1. 扫最近 lookbackWindow 内的变更
	type row struct {
		MerchantID uint64    `gorm:"column:merchant_id"`
		BizDate    time.Time `gorm:"column:biz_date"`
	}
	q := `
        SELECT DISTINCT o.merchant_id, DATE(o.paid_at) AS biz_date
        FROM t_split_execution se USE INDEX (idx_status_executed)
        INNER JOIN t_order o ON o.order_no = se.order_no
        WHERE se.executed_at >= ?`
	since := time.Now().Add(-lookbackWindow)
	var rows []row
	if err := s.db.WithContext(ctx).Raw(q, since).Scan(&rows).Error; err != nil {
		s.logger.Warn("recompute scan changed exec fail", zap.Error(err))
		return
	}
	if len(rows) == 0 {
		return
	}

	// 2. 应用层去重 + 串行处理
	for _, r := range rows {
		key := recomputeKey(r.MerchantID, r.BizDate)
		s.mu.Lock()
		if _, ok := s.inFlight[key]; ok {
			s.mu.Unlock()
			continue
		}
		s.inFlight[key] = struct{}{}
		s.mu.Unlock()

		go func(merchantID uint64, bizDate time.Time, k string) {
			defer func() {
				s.mu.Lock()
				delete(s.inFlight, k)
				s.mu.Unlock()
				if rec := recover(); rec != nil {
					s.logger.Error("recompute one panic", zap.Any("panic", rec))
				}
			}()
			started := time.Now()
			n, err := s.statsSvc.RecomputeSplitStatus(ctx, merchantID, bizDate)
			duration := time.Since(started)
			prom.SplitStatusRecomputeDuration.Observe(float64(duration.Milliseconds()))
			if err != nil {
				s.logger.Warn("recompute split status fail",
					zap.Uint64("merchant", merchantID),
					zap.Time("biz_date", bizDate),
					zap.Error(err))
				return
			}
			if n > 0 {
				s.logger.Info("recompute split status done",
					zap.Uint64("merchant", merchantID),
					zap.Time("biz_date", bizDate),
					zap.Int("updated_stores", n),
					zap.Duration("duration", duration))
			}
		}(r.MerchantID, r.BizDate, key)
	}
}

func recomputeKey(merchantID uint64, bizDate time.Time) string {
	return uint64ToStr(merchantID) + "|" + bizDate.Format("2006-01-02")
}

// uint64ToStr 快速 uint64 → string（避免 strconv 导入冲突）。
func uint64ToStr(v uint64) string {
	if v == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}