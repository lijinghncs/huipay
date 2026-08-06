// 包 handler 暴露账户/钱包 HTTP 接口。
package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/huipay/huipay-backend/infra/errs"
	"github.com/huipay/huipay-backend/internal/account/service"
)

// Handler 账户 Handler。
type Handler struct {
	svc    *service.Service
	logger *zap.Logger
}

// New 构造 Handler。
func New(svc *service.Service, logger *zap.Logger) *Handler {
	return &Handler{svc: svc, logger: logger}
}

// GetWallet GET /v1/wallets/:entity_id。
func (h *Handler) GetWallet(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("entity_id"), 10, 64)
	if err != nil {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, "entity_id invalid", 200))
		return
	}
	w, err := h.svc.GetWallet(c.Request.Context(), id)
	if err != nil {
		_ = c.Error(err)
		return
	}
	errs.OK(c, w)
}

// ListEntries GET /v1/wallets/:entity_id/entries。
func (h *Handler) ListEntries(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("entity_id"), 10, 64)
	if err != nil {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, "entity_id invalid", 200))
		return
	}
	limit := 50
	if v := c.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			limit = n
		}
	}
	list, err := h.svc.ListEntries(c.Request.Context(), id, limit)
	if err != nil {
		_ = c.Error(err)
		return
	}
	errs.OK(c, gin.H{"items": list, "limit": limit})
}