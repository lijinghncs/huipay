// 包 service 编排订单业务。
package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/huipay/huipay-backend/infra/errs"
	"github.com/huipay/huipay-backend/infra/prom"
	"github.com/huipay/huipay-backend/internal/domain/vo"
	"github.com/huipay/huipay-backend/internal/order/model"
	"github.com/huipay/huipay-backend/internal/order/repository"
	"github.com/huipay/huipay-backend/internal/payment/channel"
	"github.com/huipay/huipay-backend/internal/payment/router"
)

// PrecreateRequest 预下单请求。
type PrecreateRequest struct {
	MerchantID      uint64
	MerchantOrderNo string
	Amount          int64 // 分
	Subject         string
	NotifyURL       string
	ExpireSeconds   int
}

// PrecreateResponse 预下单响应。
type PrecreateResponse struct {
	OrderNo    string             `json:"order_no"`
	Channels   []ChannelAvailable `json:"channels"`
	ExpireAt   time.Time          `json:"expire_at"`
	CheckoutURL string             `json:"checkout_url"`
}

// ChannelAvailable 可用通道。
type ChannelAvailable struct {
	Code      vo.ChannelCode `json:"code"`
	FeeRate   string         `json:"fee_rate"`
	Available bool           `json:"available"`
}

// PayRequest 发起支付请求。
type PayRequest struct {
	OrderNo string
	PayType channel.PayType // 支付场景；空则默认 NATIVE
	OpenID  string          // JSAPI 场景必填
}

// PayResponse 发起支付响应（场景化支付参数）。
type PayResponse struct {
	OrderNo  string          `json:"order_no"`
	Channel  vo.ChannelCode  `json:"channel"`
	PayType  channel.PayType `json:"pay_type"`
	PayURL   string          `json:"pay_url,omitempty"`   // H5 跳转地址
	QRCode   string          `json:"qr_code,omitempty"`   // Native 扫码内容
	PrepayID string          `json:"prepay_id,omitempty"` // JSAPI 预支付单号
}

// Service 订单服务。
type Service struct {
	db     *gorm.DB
	repo   *repository.OrderRepo
	logger *zap.Logger
	router *router.Router
}

// NewService 构造 Service。
func NewService(db *gorm.DB, logger *zap.Logger, router *router.Router) *Service {
	return &Service{db: db, repo: repository.NewOrderRepo(db), logger: logger, router: router}
}

// Precreate 预下单（含幂等）。
func (s *Service) Precreate(ctx context.Context, req *PrecreateRequest) (*PrecreateResponse, error) {
	if req.Amount <= 0 {
		return nil, errs.New(errs.CodeInvalidParams, "amount must be positive", 200)
	}
	if req.MerchantOrderNo == "" {
		return nil, errs.New(errs.CodeInvalidParams, "merchant_order_no required", 200)
	}

	// 幂等：商户号 + 商户单号
	exists, err := s.repo.GetByMerchantOrder(ctx, req.MerchantID, req.MerchantOrderNo)
	if err != nil {
		return nil, err
	}
	if exists != nil {
		prom.IdempotentHit.Inc()
		return s.toResp(exists), nil
	}

	orderNo := "HP" + time.Now().UTC().Format("20060102") + uuid.NewString()[:16]
	expire := time.Now().Add(time.Duration(req.ExpireSeconds) * time.Second)
	if req.ExpireSeconds == 0 {
		expire = time.Now().Add(15 * time.Minute)
	}

	m := &model.OrderModel{
		OrderNo:         orderNo,
		MerchantOrderNo: req.MerchantOrderNo,
		MerchantID:      req.MerchantID,
		OrderType:       "PAYMENT",
		Amount:          req.Amount,
		PaidAmount:      0,
		SplitStatus:     "PENDING",
		Status:          "CREATED",
		ExpireAt:        &expire,
	}
	if err := s.repo.Create(ctx, m); err != nil {
		return nil, fmt.Errorf("create order: %w", err)
	}
	prom.OrderCreateTotal.Inc()
	return s.toResp(m), nil
}

// GetByOrderNo 按订单号查询。
func (s *Service) GetByOrderNo(ctx context.Context, orderNo string) (*model.OrderModel, error) {
	m, err := s.repo.GetByOrderNo(ctx, orderNo)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return m, nil
}

// Pay 发起支付：路由决策 → 调通道预下单 → 返回场景化支付参数。
func (s *Service) Pay(ctx context.Context, req *PayRequest) (*PayResponse, error) {
	order, err := s.GetByOrderNo(ctx, req.OrderNo)
	if err != nil {
		return nil, err
	}
	if order == nil {
		return nil, errs.New(errs.CodeInvalidParams, "order not found", 400)
	}
	if order.Status == string(vo.OrderPaid) {
		return nil, errs.New(errs.CodeInvalidParams, "order already paid", 400)
	}
	if order.Status != string(vo.OrderCreated) {
		return nil, errs.New(errs.CodeInvalidParams, "order not payable", 400)
	}

	payType := req.PayType
	if payType == "" {
		payType = channel.PayTypeNative
	}

	// 路由决策（未指定通道时按可用性选择）
	dec, err := s.router.Route(ctx, &router.Request{
		MerchantID: order.MerchantID,
		Amount:     order.Amount,
	})
	if err != nil {
		return nil, errs.Wrap(errs.CodeChannelUnavailable, "no available channel", 400, err)
	}
	adapter := s.router.GetAdapter(dec.Channel)
	if adapter == nil {
		return nil, errs.New(errs.CodeChannelUnavailable, "channel unavailable", 400)
	}

	expireSecs := 0
	if order.ExpireAt != nil {
		if remain := int(time.Until(*order.ExpireAt).Seconds()); remain > 0 {
			expireSecs = remain
		}
	}

	cp, err := adapter.CreatePayment(ctx, &channel.CreatePaymentRequest{
		OrderNo:    order.OrderNo,
		Amount:     order.Amount,
		Subject:    "订单 " + order.OrderNo,
		ExpireSecs: expireSecs,
		PayType:    payType,
		OpenID:     req.OpenID,
	})
	if err != nil {
		return nil, errs.Wrap(errs.CodeChannelUnavailable, "create payment fail", 400, err)
	}

	return &PayResponse{
		OrderNo:  order.OrderNo,
		Channel:  dec.Channel,
		PayType:  payType,
		PayURL:   cp.PayURL,
		QRCode:   cp.QRCode,
		PrepayID: cp.PrepayID,
	}, nil
}

// MarkPaid 标记订单支付成功（通道回调时调用）。
// 通过条件更新（仅 CREATED 状态）保证幂等：返回 true 表示本次真正入账，
// false 表示订单已处理（重复回调命中），避免重复入账。
func (s *Service) MarkPaid(ctx context.Context, orderNo string, paidAmount int64, channel vo.ChannelCode, channelTradeNo string) (bool, error) {
	now := time.Now()
	res := s.db.WithContext(ctx).
		Model(&model.OrderModel{}).
		Where("order_no = ? AND status = ?", orderNo, string(vo.OrderCreated)).
		Updates(map[string]any{
			"status":           string(vo.OrderPaid),
			"paid_amount":      paidAmount,
			"channel":          channel,
			"channel_trade_no": channelTradeNo,
			"paid_at":          now,
		})
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
}

// ListRequest 订单分页查询请求。
type ListRequest struct {
	MerchantID uint64
	Status     string // 空 = 全部状态
	Page       int
	Size       int
}

// ListResponse 订单分页查询响应。
type ListResponse struct {
	Items []model.OrderModel `json:"items"`
	Total int64              `json:"total"`
}

// ListOrders 按商户号分页查询订单列表。
func (s *Service) ListOrders(ctx context.Context, req *ListRequest) (*ListResponse, error) {
	rows, total, err := s.repo.ListByMerchant(ctx, req.MerchantID, req.Status, req.Page, req.Size)
	if err != nil {
		return nil, err
	}
	return &ListResponse{Items: rows, Total: total}, nil
}

// EmbedInfoResponse 收银台 embed 预下单信息（简版，不返回 token）。
type EmbedInfoResponse struct {
	OrderNo  string             `json:"order_no"`
	Channels []ChannelAvailable `json:"channels"`
	Amount   int64              `json:"amount"`
	Discount int64              `json:"discount"`
}

// EmbedInfo 查询订单预下单信息（供收银台 embed 页面展示）。
func (s *Service) EmbedInfo(ctx context.Context, orderNo string) (*EmbedInfoResponse, error) {
	order, err := s.GetByOrderNo(ctx, orderNo)
	if err != nil {
		return nil, err
	}
	if order == nil {
		return nil, errs.New(errs.CodeInvalidParams, "order not found", 400)
	}
	return &EmbedInfoResponse{
		OrderNo: order.OrderNo,
		Channels: []ChannelAvailable{
			{Code: vo.ChannelWeChat, FeeRate: "0.60%", Available: true},
			{Code: vo.ChannelAlipay, FeeRate: "0.55%", Available: true},
		},
		Amount:   order.Amount,
		Discount: order.CouponDiscount,
	}, nil
}

func (s *Service) toResp(m *model.OrderModel) *PrecreateResponse {
	return &PrecreateResponse{
		OrderNo: m.OrderNo,
		Channels: []ChannelAvailable{
			{Code: vo.ChannelWeChat, FeeRate: "0.60%", Available: true},
			{Code: vo.ChannelAlipay, FeeRate: "0.55%", Available: true},
		},
		ExpireAt:    *m.ExpireAt,
		CheckoutURL: fmt.Sprintf("https://checkout.huipay.cn/h5?order=%s", m.OrderNo),
	}
}