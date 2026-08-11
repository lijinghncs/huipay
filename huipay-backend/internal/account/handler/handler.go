// 包 handler 暴露账户/钱包 HTTP 接口。
package handler

import (
	"strconv"
	"time"

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
// 支持过滤参数：biz_type / biz_id / start / end / page / size（均为可选，保持向后兼容）。
func (h *Handler) ListEntries(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("entity_id"), 10, 64)
	if err != nil {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, "entity_id invalid", 200))
		return
	}
	page := 1
	if v := c.Query("page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			page = n
		}
	}
	size := 50
	if v := c.Query("size"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			size = n
		}
	} else if v := c.Query("limit"); v != "" { // 向后兼容旧参数
		if n, err := strconv.Atoi(v); err == nil {
			size = n
		}
	}
	q := service.EntryQuery{
		BizType: c.Query("biz_type"),
		BizID:   c.Query("biz_id"),
		Page:    page,
		Size:    size,
	}
	if v := c.Query("start"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			q.Start = &t
		}
	}
	if v := c.Query("end"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			q.End = &t
		}
	}
	res, err := h.svc.ListEntriesFiltered(c.Request.Context(), id, q)
	if err != nil {
		_ = c.Error(err)
		return
	}
	errs.OK(c, res)
}