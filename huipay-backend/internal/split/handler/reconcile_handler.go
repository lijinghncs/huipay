package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/huipay/huipay-backend/infra/errs"
)

// ListExceptions GET /v1/merchant/split/exceptions（差错中心：异常订单聚合）。
func (h *Handler) ListExceptions(c *gin.Context) {
	merchantID := c.GetUint64("merchant_id")
	if merchantID == 0 {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, "merchant_id required", 200))
		return
	}
	page, size := parsePageQuery(c)
	var degraded *int
	if d := c.Query("degraded"); d != "" {
		if v, err := strconv.Atoi(d); err == nil {
			degraded = &v
		}
	}
	resp, err := h.svc.ListExceptions(c.Request.Context(), merchantID, c.Query("status"), degraded, page, size)
	if err != nil {
		_ = c.Error(err)
		return
	}
	errs.OK(c, resp)
}

// ListAudits GET /v1/merchant/split/audit?biz_type=&biz_id=（biz_id 必填，商户隔离）。
func (h *Handler) ListAudits(c *gin.Context) {
	merchantID := c.GetUint64("merchant_id")
	if merchantID == 0 {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, "merchant_id required", 200))
		return
	}
	page, size := parsePageQuery(c)
	resp, err := h.svc.ListAudits(c.Request.Context(), merchantID, c.Query("biz_type"), c.Query("biz_id"), page, size)
	if err != nil {
		_ = c.Error(err)
		return
	}
	errs.OK(c, resp)
}

// ListReconcileDiffs GET /v1/merchant/split/reconcile-diffs（对账差异，商户隔离）。
func (h *Handler) ListReconcileDiffs(c *gin.Context) {
	merchantID := c.GetUint64("merchant_id")
	if merchantID == 0 {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, "merchant_id required", 200))
		return
	}
	start, end, ok := parseDateRange(c, "start_date", "end_date")
	if !ok {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, "start_date and end_date required (YYYY-MM-DD)", 200))
		return
	}
	end = end.AddDate(0, 0, 1)
	var resolved *bool
	if r := c.Query("resolved"); r != "" {
		v := r == "1" || r == "true"
		resolved = &v
	}
	page, size := parsePageQuery(c)
	resp, err := h.svc.ListReconcileDiffs(c.Request.Context(), merchantID, c.Query("diff_type"), resolved, start, end, page, size)
	if err != nil {
		_ = c.Error(err)
		return
	}
	errs.OK(c, resp)
}

// ResolveReconcileDiff POST /v1/merchant/split/reconcile-diffs/:id/resolve（差异核销）。
func (h *Handler) ResolveReconcileDiff(c *gin.Context) {
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
	if err := h.svc.ResolveReconcileDiff(c.Request.Context(), merchantID, id); err != nil {
		_ = c.Error(err)
		return
	}
	errs.OK(c, gin.H{"id": id, "resolved": true})
}