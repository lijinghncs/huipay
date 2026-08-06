// 包 router 实现支付路由三层决策：合规 → 成本 → 成功率。
package router

import (
	"context"

	"github.com/huipay/huipay-backend/internal/domain/vo"
	"github.com/huipay/huipay-backend/internal/payment/channel"
)

// Request 路由决策输入。
type Request struct {
	MerchantID uint64
	Amount     int64
	Channel    vo.ChannelCode // 可选：商户指定通道
}

// Decision 路由决策输出。
type Decision struct {
	Channel vo.ChannelCode
	Reason  string
}

// Router 路由决策器。
type Router struct {
	adapters map[vo.ChannelCode]channel.Adapter
}

// NewDefaultRouter 构造默认路由（注册微信 + 支付宝）。
func NewDefaultRouter() *Router {
	r := &Router{adapters: map[vo.ChannelCode]channel.Adapter{}}
	r.adapters[vo.ChannelWeChat] = nil // 占位：实际项目注入 wechat.New()
	r.adapters[vo.ChannelAlipay] = nil // 占位：实际项目注入 alipay.New()
	return r
}

// Register 注册适配器。
func (r *Router) Register(a channel.Adapter) { r.adapters[a.Code()] = a }

// Route 决策（骨架：优先返回微信）。
func (r *Router) Route(ctx context.Context, req *Request) (*Decision, error) {
	if req.Channel != "" {
		if _, ok := r.adapters[req.Channel]; ok {
			return &Decision{Channel: req.Channel, Reason: "merchant specified"}, nil
		}
	}
	// TODO: 合规过滤（制裁/区域/IP）→ 成本优先 → 成功率优先
	return &Decision{Channel: vo.ChannelWeChat, Reason: "default"}, nil
}