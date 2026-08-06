package vo

// ChannelCode 支付通道编码。
type ChannelCode string

const (
	ChannelWeChat   ChannelCode = "WECHAT"
	ChannelAlipay   ChannelCode = "ALIPAY"
	ChannelUnionPay ChannelCode = "UNIONPAY"
	ChannelBank     ChannelCode = "BANK"
	ChannelDCEP     ChannelCode = "DCEP"
)

// OrderStatus 订单状态。
type OrderStatus string

const (
	OrderCreated OrderStatus = "CREATED"
	OrderPaid    OrderStatus = "PAID"
	OrderClosed  OrderStatus = "CLOSED"
	OrderRefunded OrderStatus = "REFUNDED"
)

// SplitStatus 分账执行状态。
type SplitStatus string

const (
	SplitPending    SplitStatus = "PENDING"
	SplitProcessing SplitStatus = "PROCESSING"
	SplitSuccess    SplitStatus = "SUCCESS"
	SplitFailed     SplitStatus = "FAILED"
	SplitReturned   SplitStatus = "RETURNED"
)

// EntityType 主体类型。
type EntityType string

const (
	EntityMerchant EntityType = "MERCHANT"
	EntityStore    EntityType = "STORE"
	EntityPromoter EntityType = "PROMOTER"
	EntityPlatform EntityType = "PLATFORM"
	EntityISV      EntityType = "ISV"
)