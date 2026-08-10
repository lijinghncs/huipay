// 包 channel 定义支付通道适配器接口。
package channel

import (
	"context"

	"github.com/huipay/huipay-backend/internal/domain/vo"
)

// PayType 预下单支付方式/场景。
type PayType string

const (
	PayTypeNative PayType = "NATIVE" // 扫码支付（PC/收银台，返回 code_url）
	PayTypeH5     PayType = "H5"     // 移动端网页支付（返回 h5_url 跳转）
	PayTypeJSAPI  PayType = "JSAPI"  // 微信内拉起（返回 prepay_id）
)

// CreatePaymentRequest 预下单请求。
type CreatePaymentRequest struct {
	OrderNo    string
	Amount     int64
	Subject    string
	NotifyURL  string
	ExpireSecs int
	PayType    PayType // 支付场景，默认 NATIVE
	OpenID     string  // JSAPI 场景必填
}

// CreatePaymentResponse 预下单响应。
type CreatePaymentResponse struct {
	ChannelTradeNo string
	PayURL         string
	QRCode         string
	PrepayID       string // JSAPI 预支付单号，用于前端拉起
}

// PaymentStatus 支付状态。
type PaymentStatus struct {
	ChannelTradeNo string
	Paid           bool
	PaidAmount     int64
	PaidAt         int64
}

// RefundRequest 退款请求。
type RefundRequest struct {
	OrderNo       string
	RefundNo      string
	RefundAmount  int64
	TotalAmount   int64
	Reason        string
}

// RefundResponse 退款响应。
type RefundResponse struct {
	ChannelRefundNo string
}

// SplitRequest 分账请求。
type SplitRequest struct {
	OrderNo  string
	Receivers []Receiver
}

// Receiver 分账接收方。
type Receiver struct {
	EntityID uint64
	Amount   int64
}

// SplitResponse 分账响应。
type SplitResponse struct {
	ChannelSplitNo string
}

// ReturnSplitRequest 回退分账请求。
type ReturnSplitRequest struct {
	OrderNo  string
	Receiver Receiver
}

// FinishSplitRequest 完结分账请求。
type FinishSplitRequest struct {
	OrderNo string
}

// NotifyPayload 回调载荷。
type NotifyPayload struct {
	OrderNo        string
	ChannelTradeNo string
	NotifyID       string // 微信回调唯一 notify_id（幂等键用）
	Paid           bool
	PaidAmount     int64
	Raw            []byte
}

// Adapter 通道适配器接口。
type Adapter interface {
	Code() vo.ChannelCode
	CreatePayment(ctx context.Context, req *CreatePaymentRequest) (*CreatePaymentResponse, error)
	QueryPayment(ctx context.Context, channelTradeNo string) (*PaymentStatus, error)
	Refund(ctx context.Context, req *RefundRequest) (*RefundResponse, error)
	Split(ctx context.Context, req *SplitRequest) (*SplitResponse, error)
	ReturnSplit(ctx context.Context, req *ReturnSplitRequest) error
	FinishSplit(ctx context.Context, req *FinishSplitRequest) error
	CloseOrder(ctx context.Context, orderNo string) error
	VerifyNotify(ctx context.Context, raw []byte, headers map[string]string) (*NotifyPayload, error)
	// VerifyAndDecrypt 仅验签 + 解密，返回明文（供退款等非交易回调复用）。
	VerifyAndDecrypt(ctx context.Context, raw []byte, headers map[string]string) ([]byte, error)
}