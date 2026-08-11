// 包 handler 暴露订单 HTTP 接口。
package handler

import (
	"strconv"
	"time"

	"go.uber.org/zap"

	"github.com/huipay/huipay-backend/infra/errs"
	"github.com/huipay/huipay-backend/internal/order/service"
	"github.com/huipay/huipay-backend/internal/payment/channel"
	"github.com/gin-gonic/gin"
)

// Handler 订单 Handler。
type Handler struct {
	svc    *service.Service
	logger *zap.Logger
}

// New 构造 Handler。
func New(svc *service.Service, logger *zap.Logger) *Handler {
	return &Handler{svc: svc, logger: logger}
}

// precreateReqHTTP HTTP 层请求结构。
type precreateReqHTTP struct {
	MerchantID      uint64 `json:"merchant_id" binding:"required"`
	MerchantOrderNo string `json:"merchant_order_no" binding:"required"`
	Amount          int64  `json:"amount" binding:"required,gt=0"`
	Subject         string `json:"subject"`
	NotifyURL       string `json:"notify_url"`
	ExpireSeconds   int    `json:"expire_seconds"`
}

// Precreate POST /v1/checkout/precreate。
func (h *Handler) Precreate(c *gin.Context) {
	var req precreateReqHTTP
	if err := c.ShouldBindJSON(&req); err != nil {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, err.Error(), 200))
		return
	}
	// 优先使用 X-Merchant-Id 头注入的商户号（覆盖 body），打通真实商户链路
	if mid := c.GetUint64("merchant_id"); mid > 0 {
		req.MerchantID = mid
	}
	resp, err := h.svc.Precreate(c.Request.Context(), &service.PrecreateRequest{
		MerchantID:      req.MerchantID,
		MerchantOrderNo: req.MerchantOrderNo,
		Amount:          req.Amount,
		Subject:         req.Subject,
		NotifyURL:       req.NotifyURL,
		ExpireSeconds:   req.ExpireSeconds,
	})
	if err != nil {
		_ = c.Error(err)
		return
	}
	errs.OK(c, resp)
}

// PrecreateByCode POST /v1/checkout/precreate-by-code
// 消费者扫码牌后输入金额建单。入参 code_id + amount，后端反查码牌所属商户。
func (h *Handler) PrecreateByCode(c *gin.Context) {
	var req service.PrecreateByCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, err.Error(), 200))
		return
	}
	resp, err := h.svc.PrecreateByCode(c.Request.Context(), &req)
	if err != nil {
		_ = c.Error(err)
		return
	}
	errs.OK(c, resp)
}

// Get GET /v1/checkout/:order_no。
func (h *Handler) Get(c *gin.Context) {
	no := c.Param("order_no")
	if no == "" {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, "order_no required", 200))
		return
	}
	m, err := h.svc.GetByOrderNo(c.Request.Context(), no)
	if err != nil {
		_ = c.Error(err)
		return
	}
	if m == nil {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, "order not found", 200))
		return
	}
	errs.OK(c, m)
}

// payReqHTTP 发起支付 HTTP 请求结构。
type payReqHTTP struct {
	PayType string `json:"pay_type"`            // NATIVE / H5 / JSAPI
	OpenID  string `json:"openid,omitempty"`    // JSAPI 场景必填
}

// Query GET /v1/checkout/:order_no/query
// 主动查询支付结果（只读，不改订单状态，以回调为准）。
func (h *Handler) Query(c *gin.Context) {
	no := c.Param("order_no")
	if no == "" {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, "order_no required", 200))
		return
	}
	res, err := h.svc.QueryPayment(c.Request.Context(), no)
	if err != nil {
		_ = c.Error(err)
		return
	}
	errs.OK(c, res)
}

// Pay POST /v1/checkout/:order_no/pay。
func (h *Handler) Pay(c *gin.Context) {
	no := c.Param("order_no")
	if no == "" {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, "order_no required", 200))
		return
	}
	var req payReqHTTP
	if err := c.ShouldBindJSON(&req); err != nil {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, err.Error(), 200))
		return
	}
	resp, err := h.svc.Pay(c.Request.Context(), &service.PayRequest{
		OrderNo: no,
		PayType: channel.PayType(req.PayType),
		OpenID:  req.OpenID,
	})
	if err != nil {
		_ = c.Error(err)
		return
	}
	errs.OK(c, resp)
}

// Refund POST /v1/checkout/:order_no/refund（骨架，后续接入退款服务）。
func (h *Handler) Refund(c *gin.Context) {
	no := c.Param("order_no")
	h.logger.Info("refund requested", zap.String("order_no", no))
	errs.OK(c, gin.H{"order_no": no, "status": "REFUND_PENDING"})
}

// List GET /v1/checkout/list。
// 查询参数：status / code_id / channel / start / end（均可空）、page（默认 1）、size（默认 20，上限 100）。
// 商户号取自 X-Merchant-Id 中间件注入的上下文。
func (h *Handler) List(c *gin.Context) {
	merchantID := c.GetUint64("merchant_id")
	if merchantID == 0 {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, "merchant_id required", 200))
		return
	}
	page, size := parsePageSize(c)
	req := &service.ListRequest{
		MerchantID: merchantID,
		Status:     c.Query("status"),
		CodeID:     c.Query("code_id"),
		Channel:    c.Query("channel"),
		Page:       page,
		Size:       size,
	}
	if v := c.Query("start"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			req.Start = &t
		}
	}
	if v := c.Query("end"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			req.End = &t
		}
	}
	resp, err := h.svc.ListOrders(c.Request.Context(), req)
	if err != nil {
		_ = c.Error(err)
		return
	}
	errs.OK(c, resp)
}

// parsePageSize 解析 page/size 查询参数，非法值回落默认。
func parsePageSize(c *gin.Context) (page, size int) {
	page, _ = strconv.Atoi(c.Query("page"))
	size, _ = strconv.Atoi(c.Query("size"))
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 20
	}
	if size > 100 {
		size = 100
	}
	return page, size
}

// EmbedInfo POST /v1/checkout/embed-info。
// 入参：order_no；产出预下单信息（简版，不返回 token；token 化放 P5 OAuth）。
func (h *Handler) EmbedInfo(c *gin.Context) {
	var req struct {
		OrderNo string `json:"order_no" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, err.Error(), 200))
		return
	}
	resp, err := h.svc.EmbedInfo(c.Request.Context(), req.OrderNo)
	if err != nil {
		_ = c.Error(err)
		return
	}
	errs.OK(c, resp)
}
