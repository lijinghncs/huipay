// 包 handler 暴露订单 HTTP 接口。
package handler

import (
	"go.uber.org/zap"

	"github.com/huipay/huipay-backend/infra/errs"
	"github.com/huipay/huipay-backend/internal/order/service"
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

// Refund POST /v1/checkout/:order_no/refund（骨架，后续接入退款服务）。
func (h *Handler) Refund(c *gin.Context) {
	no := c.Param("order_no")
	h.logger.Info("refund requested", zap.String("order_no", no))
	errs.OK(c, gin.H{"order_no": no, "status": "REFUND_PENDING"})
}