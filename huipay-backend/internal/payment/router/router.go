// 包 router 实现支付路由决策与通道获取。
// 迭代 3：注入真实适配器，按通道可用性做决策，并暴露 GetAdapter 供业务调用。
package router

import (
	"context"
	"fmt"

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

// NewDefaultRouter 构造空路由（由 main 注入已启用的适配器）。
func NewDefaultRouter() *Router {
	return &Router{adapters: map[vo.ChannelCode]channel.Adapter{}}
}

// Register 注册适配器。
func (r *Router) Register(a channel.Adapter) { r.adapters[a.Code()] = a }

// GetAdapter 按通道编码获取已注入的适配器；未启用返回 nil。
func (r *Router) GetAdapter(code vo.ChannelCode) channel.Adapter { return r.adapters[code] }

// Available 判断指定通道是否已注入（可用）。
func (r *Router) Available(code vo.ChannelCode) bool {
	return r.adapters[code] != nil
}

// Route 决策：商户指定优先，否则按可用通道默认选择（微信优先，支付宝兜底）。
func (r *Router) Route(ctx context.Context, req *Request) (*Decision, error) {
	if req.Channel != "" {
		if a := r.adapters[req.Channel]; a != nil {
			return &Decision{Channel: req.Channel, Reason: "merchant specified"}, nil
		}
		return nil, fmt.Errorf("router: channel %s not available", req.Channel)
	}
	// 默认策略：微信优先，支付宝兜底
	if r.adapters[vo.ChannelWeChat] != nil {
		return &Decision{Channel: vo.ChannelWeChat, Reason: "default"}, nil
	}
	if r.adapters[vo.ChannelAlipay] != nil {
		return &Decision{Channel: vo.ChannelAlipay, Reason: "fallback"}, nil
	}
	return nil, fmt.Errorf("router: no available channel")
}