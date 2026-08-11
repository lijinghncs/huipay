// 包 scheduler 提供进程内定时任务。
// 超时关单：扫描过期未支付订单，调用通道关单并 CAS 更新订单状态。
package scheduler

import (
	"context"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/huipay/huipay-backend/internal/domain/vo"
	"github.com/huipay/huipay-backend/internal/order/model"
	"github.com/huipay/huipay-backend/internal/payment/channel"
	"github.com/huipay/huipay-backend/internal/payment/router"
)

// MerchantAdapterResolver 按商户解析通道适配器（商户级微信配置生效；nil 表示走平台通道）。
type MerchantAdapterResolver interface {
	GetAdapter(ctx context.Context, merchantID uint64, code vo.ChannelCode) (channel.Adapter, error)
}

// CloseExpiredScheduler 超时关单调度器。
type CloseExpiredScheduler struct {
	db               *gorm.DB
	router           *router.Router
	merchantAdapters MerchantAdapterResolver
	logger           *zap.Logger
	interval         time.Duration // 本轮写死 30 秒，不走配置
}

// NewCloseExpiredScheduler 构造调度器。
func NewCloseExpiredScheduler(db *gorm.DB, r *router.Router, merchantAdapters MerchantAdapterResolver, interval time.Duration, logger *zap.Logger) *CloseExpiredScheduler {
	return &CloseExpiredScheduler{db: db, router: r, merchantAdapters: merchantAdapters, logger: logger, interval: interval}
}

// Start 启动定时扫描，阻塞运行直到 ctx 取消。
func (s *CloseExpiredScheduler) Start(ctx context.Context) {
	ticker := time.NewTicker(s.interval)
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

func (s *CloseExpiredScheduler) runOnce(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			s.logger.Error("close expired scheduler panic", zap.Any("panic", r))
		}
	}()

	var rows []model.OrderModel
	if err := s.db.WithContext(ctx).
		Select("order_no", "merchant_id", "channel").
		Where("status = ? AND expire_at < ?", string(vo.OrderCreated), time.Now()).
		Limit(100).
		Find(&rows).Error; err != nil {
		s.logger.Error("close expired scan fail", zap.Error(err))
		return
	}

	for _, o := range rows {
		// 先调通道关单（失败仅 log，不影响 DB 关单）
		if o.Channel != "" {
			a := s.router.GetAdapter(o.Channel)
			if s.merchantAdapters != nil {
				if ma, err := s.merchantAdapters.GetAdapter(ctx, o.MerchantID, o.Channel); err != nil {
					s.logger.Warn("resolve merchant adapter for close fail, fallback platform",
						zap.Uint64("merchant_id", o.MerchantID),
						zap.String("channel", string(o.Channel)), zap.Error(err))
				} else if ma != nil {
					a = ma
				}
			}
			if a != nil {
				if err := a.CloseOrder(ctx, o.OrderNo); err != nil {
					s.logger.Warn("close order channel call fail",
						zap.String("order_no", o.OrderNo),
						zap.String("channel", string(o.Channel)), zap.Error(err))
				}
			}
		}
		// CAS 关单，防止与 notify handler 并发重复关单
		res := s.db.WithContext(ctx).
			Model(&model.OrderModel{}).
			Where("order_no = ? AND status = ?", o.OrderNo, string(vo.OrderCreated)).
			Updates(map[string]any{"status": string(vo.OrderClosed), "closed_at": time.Now()})
		if res.Error != nil {
			s.logger.Error("close order db update fail",
				zap.String("order_no", o.OrderNo), zap.Error(res.Error))
			continue
		}
		if res.RowsAffected > 0 {
			s.logger.Info("order closed by scheduler", zap.String("order_no", o.OrderNo))
		}
	}
}
