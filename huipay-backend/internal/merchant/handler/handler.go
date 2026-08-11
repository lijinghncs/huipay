// 包 handler 暴露商户进件与列表 HTTP 接口。
package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/huipay/huipay-backend/infra/errs"
	merchantservice "github.com/huipay/huipay-backend/internal/merchant/service"
)

// Handler 商户 Handler。
type Handler struct {
	svc    *merchantservice.Service
	logger *zap.Logger
}

// New 构造 Handler。
func New(svc *merchantservice.Service, logger *zap.Logger) *Handler {
	return &Handler{svc: svc, logger: logger}
}

// Onboard POST /v1/admin/merchants 商户进件。
func (h *Handler) Onboard(c *gin.Context) {
	var req merchantservice.OnboardRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, "invalid request body", 200))
		return
	}
	m, err := h.svc.Onboard(c.Request.Context(), &req)
	if err != nil {
		_ = c.Error(err)
		return
	}
	errs.OK(c, m)
}

// List GET /v1/admin/merchants 商户列表。
func (h *Handler) List(c *gin.Context) {
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
	list, err := h.svc.List(c.Request.Context(), page, size, c.Query("keyword"), status)
	if err != nil {
		_ = c.Error(err)
		return
	}
	errs.OK(c, list)
}

// Get GET /v1/admin/merchants/:id 商户详情。
func (h *Handler) Get(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, "id invalid", 200))
		return
	}
	d, err := h.svc.Get(c.Request.Context(), id)
	if err != nil {
		_ = c.Error(err)
		return
	}
	errs.OK(c, d)
}

// Update PUT /v1/admin/merchants/:id 更新商户资料。
func (h *Handler) Update(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, "id invalid", 200))
		return
	}
	var req merchantservice.UpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, "invalid request body", 200))
		return
	}
	m, err := h.svc.Update(c.Request.Context(), id, &req)
	if err != nil {
		_ = c.Error(err)
		return
	}
	errs.OK(c, m)
}

// SetStatus POST /v1/admin/merchants/:id/status 启用 / 停用。
func (h *Handler) SetStatus(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, "id invalid", 200))
		return
	}
	var req struct {
		Status int `json:"status"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, "invalid request body", 200))
		return
	}
	m, err := h.svc.SetStatus(c.Request.Context(), id, req.Status)
	if err != nil {
		_ = c.Error(err)
		return
	}
	errs.OK(c, m)
}

// SelfProfile GET /v1/merchant/profile 当前商户自助详情（读中间件 merchant_id）。
func (h *Handler) SelfProfile(c *gin.Context) {
	id, ok := c.Get("merchant_id")
	if !ok {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, "missing merchant id", 401))
		return
	}
	d, err := h.svc.Get(c.Request.Context(), id.(uint64))
	if err != nil {
		_ = c.Error(err)
		return
	}
	errs.OK(c, d)
}

// SelfOverview GET /v1/merchant/overview 当前商户经营概览（读中间件 merchant_id）。
func (h *Handler) SelfOverview(c *gin.Context) {
	id, ok := c.Get("merchant_id")
	if !ok {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, "missing merchant id", 401))
		return
	}
	ov, err := h.svc.Overview(c.Request.Context(), id.(uint64))
	if err != nil {
		_ = c.Error(err)
		return
	}
	errs.OK(c, ov)
}

// Overview GET /v1/admin/merchants/:id/overview 商户经营概览。
func (h *Handler) Overview(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, "id invalid", 200))
		return
	}
	ov, err := h.svc.Overview(c.Request.Context(), id)
	if err != nil {
		_ = c.Error(err)
		return
	}
	errs.OK(c, ov)
}

// GetWechatConfig GET /v1/admin/merchants/:id/wechat-config 商户微信支付配置（敏感字段仅回 configured 标记）。
func (h *Handler) GetWechatConfig(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, "id invalid", 200))
		return
	}
	v, err := h.svc.GetWechatConfig(c.Request.Context(), id)
	if err != nil {
		_ = c.Error(err)
		return
	}
	errs.OK(c, v)
}

// UpdateWechatConfig PUT /v1/admin/merchants/:id/wechat-config 更新商户微信支付配置。
func (h *Handler) UpdateWechatConfig(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, "id invalid", 200))
		return
	}
	var req merchantservice.UpdateWechatConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, "invalid request body", 200))
		return
	}
	v, err := h.svc.UpdateWechatConfig(c.Request.Context(), id, &req)
	if err != nil {
		_ = c.Error(err)
		return
	}
	errs.OK(c, v)
}