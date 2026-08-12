// 包 mock 提供本地挡板支付通道：未启用真实微信通道时，模拟支付成功，便于本地联调。
package mock

import (
	"context"
	"fmt"
	"time"

	"github.com/huipay/huipay-backend/internal/domain/vo"
	"github.com/huipay/huipay-backend/internal/payment/channel"
)

// CompletePayment 支付成功回调：由 main 注入，内部完成订单 MarkPaid 与资金入账。
type CompletePayment func(ctx context.Context, orderNo string, amount int64) error

// Adapter mock 通道适配器。
type Adapter struct {
	complete CompletePayment
}

// New 构造 mock 适配器。
func New(complete CompletePayment) *Adapter {
	return &Adapter{complete: complete}
}

// Code 通道编码。
func (a *Adapter) Code() vo.ChannelCode { return vo.ChannelWeChat }

// CreatePayment 模拟微信预下单：返回 JSAPI 调起参数，并同步完成支付（模拟微信回调成功）。
func (a *Adapter) CreatePayment(ctx context.Context, req *channel.CreatePaymentRequest) (*channel.CreatePaymentResponse, error) {
	prepayID := "mock_prepay_" + req.OrderNo
	resp := &channel.CreatePaymentResponse{
		ChannelTradeNo: "mock_trade_" + req.OrderNo,
		PrepayID:       prepayID,
		PayURL:         "",
		QRCode:         "",
		JSAPIParams: &channel.JSAPIParams{
			AppID:     "mock_appid",
			TimeStamp: fmt.Sprintf("%d", time.Now().Unix()),
			NonceStr:  "mock_nonce",
			Package:   "prepay_id=" + prepayID,
			SignType:  "RSA",
			PaySign:   "mock_paysign",
		},
	}
	// 挡板：发起支付即模拟微信已支付成功，同步完成订单入账
	if a.complete != nil {
		if err := a.complete(ctx, req.OrderNo, req.Amount); err != nil {
			return nil, err
		}
	}
	return resp, nil
}

// QueryPayment 模拟查询：始终返回已支付。
func (a *Adapter) QueryPayment(ctx context.Context, channelTradeNo string) (*channel.PaymentStatus, error) {
	return &channel.PaymentStatus{
		ChannelTradeNo: channelTradeNo,
		Paid:           true,
		PaidAmount:     0,
		PaidAt:         time.Now().Unix(),
	}, nil
}

// Refund mock：直接成功。
func (a *Adapter) Refund(ctx context.Context, req *channel.RefundRequest) (*channel.RefundResponse, error) {
	return &channel.RefundResponse{ChannelRefundNo: "mock_refund_" + req.OrderNo}, nil
}

// Split mock：直接成功。
func (a *Adapter) Split(ctx context.Context, req *channel.SplitRequest) (*channel.SplitResponse, error) {
	return &channel.SplitResponse{ChannelSplitNo: "mock_split_" + req.OrderNo}, nil
}

// ReturnSplit mock：直接成功。
func (a *Adapter) ReturnSplit(ctx context.Context, req *channel.ReturnSplitRequest) error { return nil }

// FinishSplit mock：直接成功。
func (a *Adapter) FinishSplit(ctx context.Context, req *channel.FinishSplitRequest) error { return nil }

// CloseOrder mock：直接成功。
func (a *Adapter) CloseOrder(ctx context.Context, orderNo string) error { return nil }

// VerifyNotify mock：无真实回调，返回 nil。
func (a *Adapter) VerifyNotify(ctx context.Context, raw []byte, headers map[string]string) (*channel.NotifyPayload, error) {
	return nil, fmt.Errorf("mock channel does not accept notify")
}

// VerifyAndDecrypt mock：无真实回调，返回 nil。
func (a *Adapter) VerifyAndDecrypt(ctx context.Context, raw []byte, headers map[string]string) ([]byte, error) {
	return nil, fmt.Errorf("mock channel does not accept notify")
}