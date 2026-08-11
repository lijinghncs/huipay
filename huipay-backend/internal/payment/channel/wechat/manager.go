// 包 wechat 提供微信支付 V3 通道适配器与按商户管理的适配器管理器。
// Manager 支持：商户级微信配置懒加载/缓存、配置更新后 TTL 失效、回调按商户分流。
package wechat

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/huipay/huipay-backend/infra/config"
	"github.com/huipay/huipay-backend/internal/domain/vo"
	"github.com/huipay/huipay-backend/internal/payment/channel"
)

// defaultManagerTTL 商户适配器缓存有效期。
// 配置更新后最迟 TTL 内生效（管理端可提示"约 30 秒后生效"）。
const defaultManagerTTL = 30 * time.Second

// WechatConfigProvider 提供商户微信支付运行配置（敏感字段已解密）。
// 未配置（无配置 / enabled=false）返回 (nil, nil)。
type WechatConfigProvider interface {
	GetRuntimeConfig(ctx context.Context, merchantID uint64) (*config.WeChatConfig, error)
}

// managerEntry 缓存条目。
type managerEntry struct {
	adapter *Adapter
	at      time.Time
}

// Manager 按商户懒加载/缓存微信适配器；未配置或构造失败的商户回退平台适配器。
type Manager struct {
	platform *Adapter // 平台级默认适配器（未配置商户兜底）
	provider WechatConfigProvider
	logger   *zap.Logger
	ttl      time.Duration
	cache    sync.Map // uint64(merchantID) -> *managerEntry
}

// NewManager 构造 Manager。platform 可为 nil（微信通道整体禁用时）。
func NewManager(platform *Adapter, provider WechatConfigProvider, logger *zap.Logger) *Manager {
	return &Manager{
		platform: platform,
		provider: provider,
		logger:   logger,
		ttl:      defaultManagerTTL,
	}
}

// Get 返回指定商户的微信适配器：
//   - 商户已配置微信且构造成功 → 商户级适配器（回调路径带 merchant_id）；
//   - 商户未配置 / 配置异常 / 构造失败 → 平台适配器兜底（仅告警，不阻断）；
//   - 平台适配器为空 → (nil, nil)（通道整体禁用）。
func (m *Manager) Get(ctx context.Context, merchantID uint64) (channel.Adapter, error) {
	if merchantID == 0 || m.provider == nil {
		return m.platform, nil
	}
	if v, ok := m.cache.Load(merchantID); ok {
		e := v.(*managerEntry)
		if time.Since(e.at) < m.ttl {
			return e.adapter, nil
		}
		m.cache.Delete(merchantID)
	}

	cfg, err := m.provider.GetRuntimeConfig(ctx, merchantID)
	if err != nil {
		m.logger.Warn("merchant wechat runtime config fail, fallback platform",
			zap.Uint64("merchant_id", merchantID), zap.Error(err))
		return m.platform, nil
	}
	if cfg == nil || !cfg.Enabled {
		return m.platform, nil
	}
	a, err := NewForMerchant(*cfg, merchantID)
	if err != nil {
		m.logger.Warn("merchant wechat adapter build fail, fallback platform",
			zap.Uint64("merchant_id", merchantID), zap.Error(err))
		return m.platform, nil
	}
	m.cache.Store(merchantID, &managerEntry{adapter: a, at: time.Now()})
	return a, nil
}

// GetAdapter 按商户解析指定通道的适配器（实现订单服务的按商户解析接口）。
// 仅支持微信通道；其他通道返回 nil（走平台路由默认策略）。
func (m *Manager) GetAdapter(ctx context.Context, merchantID uint64, code vo.ChannelCode) (channel.Adapter, error) {
	if code != vo.ChannelWeChat {
		return nil, nil
	}
	return m.Get(ctx, merchantID)
}

// Invalidate 使指定商户的缓存失效（配置更新后可主动调用，可选；TTL 兜底）。
func (m *Manager) Invalidate(merchantID uint64) {
	if merchantID == 0 {
		return
	}
	m.cache.Delete(merchantID)
}
