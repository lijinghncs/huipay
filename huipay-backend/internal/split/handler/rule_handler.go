package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/huipay/huipay-backend/infra/errs"
	"github.com/huipay/huipay-backend/internal/split/service"
)

// ListRules GET /v1/merchant/split/rules。
func (h *Handler) ListRules(c *gin.Context) {
	merchantID := c.GetUint64("merchant_id")
	if merchantID == 0 {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, "merchant_id required", 200))
		return
	}
	list, err := h.svc.ListRules(c.Request.Context(), merchantID)
	if err != nil {
		_ = c.Error(err)
		return
	}
	errs.OK(c, gin.H{"items": list})
}

// CreateRule POST /v1/merchant/split/rules。
func (h *Handler) CreateRule(c *gin.Context) {
	merchantID := c.GetUint64("merchant_id")
	if merchantID == 0 {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, "merchant_id required", 200))
		return
	}
	var req service.CreateRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, err.Error(), 200))
		return
	}
	r, err := h.svc.CreateRule(c.Request.Context(), merchantID, &req)
	if err != nil {
		_ = c.Error(err)
		return
	}
	errs.OK(c, r)
}

// UpdateRule PUT /v1/merchant/split/rules/:id。
func (h *Handler) UpdateRule(c *gin.Context) {
	merchantID := c.GetUint64("merchant_id")
	if merchantID == 0 {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, "merchant_id required", 200))
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, "id invalid", 200))
		return
	}
	var req service.UpdateRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, err.Error(), 200))
		return
	}
	r, err := h.svc.UpdateRule(c.Request.Context(), id, merchantID, &req)
	if err != nil {
		_ = c.Error(err)
		return
	}
	errs.OK(c, r)
}

// SetRuleStatus POST /v1/merchant/split/rules/:id/status。
func (h *Handler) SetRuleStatus(c *gin.Context) {
	merchantID := c.GetUint64("merchant_id")
	if merchantID == 0 {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, "merchant_id required", 200))
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, "id invalid", 200))
		return
	}
	var req struct {
		Status int `json:"status"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, err.Error(), 200))
		return
	}
	if err := h.svc.SetRuleStatus(c.Request.Context(), id, merchantID, req.Status); err != nil {
		_ = c.Error(err)
		return
	}
	errs.OK(c, gin.H{"id": id, "status": req.Status})
}

// DeleteRule DELETE /v1/merchant/split/rules/:id。
func (h *Handler) DeleteRule(c *gin.Context) {
	merchantID := c.GetUint64("merchant_id")
	if merchantID == 0 {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, "merchant_id required", 200))
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, "id invalid", 200))
		return
	}
	if err := h.svc.DeleteRule(c.Request.Context(), id, merchantID); err != nil {
		_ = c.Error(err)
		return
	}
	errs.OK(c, gin.H{"id": id, "deleted": true})
}