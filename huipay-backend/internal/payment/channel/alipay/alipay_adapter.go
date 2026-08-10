// 包 alipay 实现支付宝通道适配器（骨架）。
package alipay

import (
	"context"

	"github.com/huipay/huipay-backend/internal/domain/vo"
	"github.com/huipay/huipay-backend/internal/payment/channel"
)

// Adapter 支付宝适配器。
type Adapter struct{}

// New 构造支付宝适配器。
func New() *Adapter { return &Adapter{} }

// Code 通道编码。
func (a *Adapter) Code() vo.ChannelCode { return vo.ChannelAlipay }

// CreatePayment 支付宝预下单（骨架）。
func (a *Adapter) CreatePayment(ctx context.Context, req *channel.CreatePaymentRequest) (*channel.CreatePaymentResponse, error) {
	return &channel.CreatePaymentResponse{
		ChannelTradeNo: "",
		PayURL:         "https://openapi.alipay.com/gateway.do?stub=1",
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

func (a *Adapter) CloseOrder(ctx context.Context, orderNo string) error { return nil }

func (a *Adapter) VerifyAndDecrypt(ctx context.Context, raw []byte, headers map[string]string) ([]byte, error) {
	return raw, nil
}

func (a *Adapter) VerifyNotify(ctx context.Context, raw []byte, headers map[string]string) (*channel.NotifyPayload, error) {
	// TODO 接入支付宝异步通知验签
	return &channel.NotifyPayload{Raw: raw}, nil
}