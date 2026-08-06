// 包 wechat 实现微信支付通道适配器（骨架）。
package wechat

import (
	"context"

	"github.com/huipay/huipay-backend/internal/domain/vo"
	"github.com/huipay/huipay-backend/internal/payment/channel"
)

// Adapter 微信支付适配器。
type Adapter struct{}

// New 构造微信适配器。
func New() *Adapter { return &Adapter{} }

// Code 通道编码。
func (a *Adapter) Code() vo.ChannelCode { return vo.ChannelWeChat }

// CreatePayment 微信 V3 预下单（骨架）。
func (a *Adapter) CreatePayment(ctx context.Context, req *channel.CreatePaymentRequest) (*channel.CreatePaymentResponse, error) {
	return &channel.CreatePaymentResponse{
		ChannelTradeNo: "",
		PayURL:         "weixin://wxpay/bizpayurl?pr=stub",
		QRCode:         "",
	}, nil
}

func (a *Adapter) QueryPayment(ctx context.Context, channelTradeNo string) (*channel.PaymentStatus, error) {
	return &channel.PaymentStatus{ChannelTradeNo: channelTradeNo}, nil
}

func (a *Adapter) Refund(ctx context.Context, req *channel.RefundRequest) (*channel.RefundResponse, error) {
	return &channel.RefundResponse{ChannelRefundNo: ""}, nil
}

func (a *Adapter) Split(ctx context.Context, req *channel.SplitRequest) (*channel.SplitResponse, error) {
	return &channel.SplitResponse{ChannelSplitNo: ""}, nil
}

func (a *Adapter) ReturnSplit(ctx context.Context, req *channel.ReturnSplitRequest) error { return nil }
func (a *Adapter) FinishSplit(ctx context.Context, req *channel.FinishSplitRequest) error  { return nil }

func (a *Adapter) VerifyNotify(ctx context.Context, raw []byte, headers map[string]string) (*channel.NotifyPayload, error) {
	// TODO 接入微信 V3 验签（Wechatpay-Signature）
	return &channel.NotifyPayload{Raw: raw}, nil
}