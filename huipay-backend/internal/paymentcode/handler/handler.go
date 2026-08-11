// 包 handler 暴露收款码牌 HTTP 接口。
package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/huipay/huipay-backend/infra/errs"
	"github.com/huipay/huipay-backend/internal/paymentcode/service"
)

// Handler 码牌 Handler。
type Handler struct {
	svc    *service.Service
	logger *zap.Logger
}

// New 构造 Handler。
func New(svc *service.Service, logger *zap.Logger) *Handler {
	return &Handler{svc: svc, logger: logger}
}

// merchantID 从中间件取当前登录商户号（X-Merchant-Id）。
func (h *Handler) merchantID(c *gin.Context) (uint64, bool) {
	id := c.GetUint64("merchant_id")
	if id == 0 {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, "merchant_id required", 200))
		return 0, false
	}
	return id, true
}

// createReq 创建码牌请求体。
type createReq struct {
	Remark string `json:"remark"`
}

// Create POST /v1/merchant/codes 创建码牌。
func (h *Handler) Create(c *gin.Context) {
	mid, ok := h.merchantID(c)
	if !ok {
		return
	}
	var req createReq
	if err := c.ShouldBindJSON(&req); err != nil {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, "invalid request body", 200))
		return
	}
	code, err := h.svc.Create(c.Request.Context(), &service.CreateRequest{MerchantID: mid, Remark: req.Remark})
	if err != nil {
		_ = c.Error(err)
		return
	}
	errs.OK(c, code)
}

// List GET /v1/merchant/codes 码牌列表。
func (h *Handler) List(c *gin.Context) {
	mid, ok := h.merchantID(c)
	if !ok {
		return
	}
	page := 1
	if v := c.Query("page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			page = n
		}
	}
	size := 20
	if v := c.Query("page_size"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			size = n
		}
	}
	var status *int
	if v := c.Query("status"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			status = &n
		}
	}
	list, err := h.svc.List(c.Request.Context(), mid, page, size, status)
	if err != nil {
		_ = c.Error(err)
		return
	}
	errs.OK(c, list)
}

// Disable POST /v1/merchant/codes/:id/disable 停用码牌。
func (h *Handler) Disable(c *gin.Context) {
	mid, ok := h.merchantID(c)
	if !ok {
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, "id invalid", 200))
		return
	}
	if err := h.svc.Disable(c.Request.Context(), id, mid); err != nil {
		_ = c.Error(err)
		return
	}
	errs.OK(c, gin.H{"id": id, "status": 0})
}