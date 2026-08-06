// 包 channel 定义支付通道适配器接口。
package channel

import (
	"context"

	"github.com/huipay/huipay-backend/internal/domain/vo"
)

// CreatePaymentRequest 预下单请求。
type CreatePaymentRequest struct {
	OrderNo    string
	Amount     int64
	Subject    string
	NotifyURL  string
	ExpireSecs int
}

// CreatePaymentResponse 预下单响应。
type CreatePaymentResponse struct {
	ChannelTradeNo string
	PayURL         string
	QRCode         string
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
	VerifyNotify(ctx context.Context, raw []byte, headers map[string]string) (*NotifyPayload, error)
}