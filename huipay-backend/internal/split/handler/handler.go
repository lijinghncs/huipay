// 包 handler 暴露分账 HTTP 接口。
package handler

import (
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/huipay/huipay-backend/infra/errs"
	"github.com/huipay/huipay-backend/internal/split/service"
)

// Handler 分账 Handler。
type Handler struct {
	svc    *service.Service
	logger *zap.Logger
}

// New 构造 Handler。
func New(svc *service.Service, logger *zap.Logger) *Handler {
	return &Handler{svc: svc, logger: logger}
}

// Execute POST /v1/split/execute。
func (h *Handler) Execute(c *gin.Context) {
	var req service.ExecuteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, err.Error(), 200))
		return
	}
	req.TraceID = c.GetString("trace_id")
	resp, err := h.svc.Execute(c.Request.Context(), &req)
	if err != nil {
		_ = c.Error(err)
		return
	}
	errs.OK(c, resp)
}

// Get GET /v1/split/:order_no。
func (h *Handler) Get(c *gin.Context) {
	no := c.Param("order_no")
	resp, err := h.svc.Get(c.Request.Context(), no)
	if err != nil {
		_ = c.Error(err)
		return
	}
	errs.OK(c, resp)
}