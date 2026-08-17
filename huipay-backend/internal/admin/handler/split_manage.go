// 包 handler 提供管理后台分账管理 HTTP 接口（每日执行/审计/差异/重算/重置）。
package handler

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/huipay/huipay-backend/infra/errs"
	adminservice "github.com/huipay/huipay-backend/internal/admin/service"
)

// SplitManageHandler 分账管理 Handler。
type SplitManageHandler struct {
	svc    *adminservice.SplitManageService
	logger *zap.Logger
}

// NewSplitManageHandler 构造 Handler。
func NewSplitManageHandler(svc *adminservice.SplitManageService, logger *zap.Logger) *SplitManageHandler {
	return &SplitManageHandler{svc: svc, logger: logger}
}

// parseDateRange 复用 admin/store_stats 的 parseRange（YYYY-MM-DD）。
// 这里不重复定义，handler 文件同级访问会受 package 限制；用 inline 实现。
func parseDateRange(c *gin.Context) (time.Time, time.Time, bool) {
	startStr := c.Query("start_date")
	endStr := c.Query("end_date")
	if startStr == "" || endStr == "" {
		return time.Time{}, time.Time{}, false
	}
	start, err := time.ParseInLocation("2006-01-02", startStr, time.Local)
	if err != nil {
		return time.Time{}, time.Time{}, false
	}
	end, err := time.ParseInLocation("2006-01-02", endStr, time.Local)
	if err != nil {
		return time.Time{}, time.Time{}, false
	}
	end = end.AddDate(0, 0, 1)
	return start, end, true
}

// ListDailyExecutions GET /v1/admin/split/daily-executions。
func (h *SplitManageHandler) ListDailyExecutions(c *gin.Context) {
	start, end, ok := parseDateRange(c)
	if !ok {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, "start_date and end_date required (YYYY-MM-DD)", 200))
		return
	}
	merchantID := parseUintQuery(c, "merchant_id")
	status := c.Query("status")
	page := parsePage(c.Query("page"))
	size := parseSize(c.Query("page_size"))
	resp, err := h.svc.ListDailyExecutions(c.Request.Context(), adminservice.DailyExecFilter{
		MerchantID: merchantID,
		Start:      start, End: end,
		Status:   status,
		Page:     page, PageSize: size,
	})
	if err != nil {
		_ = c.Error(err)
		return
	}
	errs.OK(c, resp)
}

// GetDailyExecution GET /v1/admin/split/daily-executions/:id。
func (h *SplitManageHandler) GetDailyExecution(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, "id invalid", 200))
		return
	}
	row, err := h.svc.GetDailyExecution(c.Request.Context(), id)
	if err != nil {
		_ = c.Error(err)
		return
	}
	if row == nil {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, "daily execution not found", 200))
		return
	}
	errs.OK(c, row)
}

// ListAudits GET /v1/admin/split/audit。
func (h *SplitManageHandler) ListAudits(c *gin.Context) {
	page := parsePage(c.Query("page"))
	size := parseSize(c.Query("page_size"))
	resp, err := h.svc.ListAudits(c.Request.Context(), adminservice.AuditFilter{
		BizType: c.Query("biz_type"), BizID: c.Query("biz_id"), Action: c.Query("action"),
		Page: page, PageSize: size,
	})
	if err != nil {
		_ = c.Error(err)
		return
	}
	errs.OK(c, resp)
}

// ListDiffs GET /v1/admin/reconcile-diffs。
func (h *SplitManageHandler) ListDiffs(c *gin.Context) {
	start, end, ok := parseDateRange(c)
	if !ok {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, "start_date and end_date required (YYYY-MM-DD)", 200))
		return
	}
	merchantID := parseUintQuery(c, "merchant_id")
	diffType := c.Query("diff_type")
	page := parsePage(c.Query("page"))
	size := parseSize(c.Query("page_size"))
	resp, err := h.svc.ListDiffs(c.Request.Context(), adminservice.DiffFilter{
		MerchantID: merchantID,
		DiffType:   diffType,
		Start:      start, End: end,
		Page: page, PageSize: size,
	})
	if err != nil {
		_ = c.Error(err)
		return
	}
	errs.OK(c, resp)
}

// RecomputeStoreStats POST /v1/admin/store-stats/recompute?merchant_id=&biz_date=。
func (h *SplitManageHandler) RecomputeStoreStats(c *gin.Context) {
	merchantID := parseUintQuery(c, "merchant_id")
	bizDateStr := c.Query("biz_date")
	if merchantID == 0 || bizDateStr == "" {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, "merchant_id and biz_date required", 200))
		return
	}
	bizDate, err := time.ParseInLocation("2006-01-02", bizDateStr, time.Local)
	if err != nil {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, "biz_date invalid (YYYY-MM-DD)", 200))
		return
	}
	operatorID := c.GetUint64("admin_id")
	n, err := h.svc.RecomputeStoreStats(c.Request.Context(), merchantID, bizDate, operatorID)
	if err != nil {
		_ = c.Error(err)
		return
	}
	errs.OK(c, gin.H{"updated_stores": n})
}

// ResetStoreSplitStatus POST /v1/admin/store-stats/reset-split-status。
func (h *SplitManageHandler) ResetStoreSplitStatus(c *gin.Context) {
	merchantID := parseUintQuery(c, "merchant_id")
	storeID := parseUintQuery(c, "store_id")
	bizDateStr := c.Query("biz_date")
	if merchantID == 0 || storeID == 0 || bizDateStr == "" {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, "merchant_id, store_id, biz_date required", 200))
		return
	}
	bizDate, err := time.ParseInLocation("2006-01-02", bizDateStr, time.Local)
	if err != nil {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, "biz_date invalid (YYYY-MM-DD)", 200))
		return
	}
	operatorID := c.GetUint64("admin_id")
	ok, err := h.svc.ResetStoreSplitStatus(c.Request.Context(), merchantID, storeID, bizDate, operatorID)
	if err != nil {
		_ = c.Error(err)
		return
	}
	if !ok {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, "store stats row not found", 200))
		return
	}
	errs.OK(c, gin.H{"reset": true})
}

// ResolveExecution POST /v1/admin/split/executions/:order_no/resolve（人工核销死单/异常分账订单）。
func (h *SplitManageHandler) ResolveExecution(c *gin.Context) {
	orderNo := c.Param("order_no")
	if orderNo == "" {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, "order_no required", 200))
		return
	}
	var req struct {
		Note string `json:"note"`
	}
	_ = c.ShouldBindJSON(&req) // note 可选
	operatorID := c.GetUint64("admin_id")
	ok, err := h.svc.ResolveExecution(c.Request.Context(), orderNo, req.Note, operatorID)
	if err != nil {
		_ = c.Error(err)
		return
	}
	if !ok {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, "订单不存在或当前状态不可核销", 200))
		return
	}
	errs.OK(c, gin.H{"order_no": orderNo, "resolved": true})
}

// ListExceptions GET /v1/admin/split/exceptions 差错中心：跨商户异常订单聚合。
func (h *SplitManageHandler) ListExceptions(c *gin.Context) {
	page := parsePage(c.Query("page"))
	size := parseSize(c.Query("page_size"))
	var degraded *int
	if d := c.Query("degraded"); d != "" {
		if v, err := strconv.Atoi(d); err == nil {
			degraded = &v
		}
	}
	resp, err := h.svc.ListExceptions(c.Request.Context(), c.Query("status"), degraded, page, size)
	if err != nil {
		_ = c.Error(err)
		return
	}
	errs.OK(c, resp)
}

// ReopenExecution POST /v1/admin/split/executions/:order_no/reopen（死单复位重开）。
func (h *SplitManageHandler) ReopenExecution(c *gin.Context) {
	orderNo := c.Param("order_no")
	if orderNo == "" {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, "order_no required", 200))
		return
	}
	operatorID := c.GetUint64("admin_id")
	ok, err := h.svc.ReopenExecution(c.Request.Context(), orderNo, operatorID)
	if err != nil {
		_ = c.Error(err)
		return
	}
	if !ok {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, "订单不存在或当前状态不可复位重开", 200))
		return
	}
	errs.OK(c, gin.H{"order_no": orderNo, "reopened": true})
}

// ResolveReconcileDiff POST /v1/admin/reconcile-diffs/:id/resolve（对账差异核销，跨商户）。
func (h *SplitManageHandler) ResolveReconcileDiff(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, "id invalid", 200))
		return
	}
	operatorID := c.GetUint64("admin_id")
	ok, err := h.svc.ResolveReconcileDiff(c.Request.Context(), id, operatorID)
	if err != nil {
		_ = c.Error(err)
		return
	}
	if !ok {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, "差异不存在或已核销", 200))
		return
	}
	errs.OK(c, gin.H{"id": id, "resolved": true})
}