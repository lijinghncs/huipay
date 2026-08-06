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

// MarkPaid 标记订单支付成功（通道回调时调用）。
func (s *Service) MarkPaid(ctx context.Context, orderNo string, paidAmount int64, channel vo.ChannelCode, channelTradeNo string) error {
	now := time.Now()
	return s.db.WithContext(ctx).
		Model(&model.OrderModel{}).
		Where("order_no = ?", orderNo).
		Updates(map[string]any{
			"status":          "PAID",
			"paid_amount":     paidAmount,
			"channel":         channel,
			"channel_trade_no": channelTradeNo,
			"paid_at":         now,
		}).Error
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