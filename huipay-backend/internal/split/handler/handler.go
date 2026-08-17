// 包 handler 暴露分账 HTTP 接口。
package handler

import (
	"strconv"
	"time"

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
	// 商户隔离：merchant_id 一律取自登录上下文，忽略请求体，杜绝越权分账他人订单
	req.MerchantID = c.GetUint64("merchant_id")
	if req.MerchantID == 0 {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, "merchant_id required", 200))
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

// ExecuteByPeriod POST /v1/merchant/split/execute-period。
func (h *Handler) ExecuteByPeriod(c *gin.Context) {
	merchantID := c.GetUint64("merchant_id")
	if merchantID == 0 {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, "merchant_id required", 200))
		return
	}
	var req service.ExecuteByPeriodRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, err.Error(), 200))
		return
	}
	resp, err := h.svc.ExecuteByPeriod(c.Request.Context(), merchantID, &req)
	if err != nil {
		_ = c.Error(err)
		return
	}
	errs.OK(c, resp)
}

// GenerateBill POST /v1/merchant/split/bills（生成分账单，待审批）。
func (h *Handler) GenerateBill(c *gin.Context) {
	merchantID := c.GetUint64("merchant_id")
	if merchantID == 0 {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, "merchant_id required", 200))
		return
	}
	var req service.ExecuteByPeriodRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, err.Error(), 200))
		return
	}
	resp, err := h.svc.GenerateBill(c.Request.Context(), merchantID, &req)
	if err != nil {
		_ = c.Error(err)
		return
	}
	errs.OK(c, resp)
}

// ListBills GET /v1/merchant/split/bills。
func (h *Handler) ListBills(c *gin.Context) {
	merchantID := c.GetUint64("merchant_id")
	if merchantID == 0 {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, "merchant_id required", 200))
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "20"))
	resp, err := h.svc.ListBills(c.Request.Context(), merchantID, page, size)
	if err != nil {
		_ = c.Error(err)
		return
	}
	errs.OK(c, resp)
}

// GetBill GET /v1/merchant/split/bills/:batch_no。
func (h *Handler) GetBill(c *gin.Context) {
	merchantID := c.GetUint64("merchant_id")
	if merchantID == 0 {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, "merchant_id required", 200))
		return
	}
	batchNo := c.Param("batch_no")
	if batchNo == "" {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, "batch_no required", 200))
		return
	}
	resp, err := h.svc.GetBillDetail(c.Request.Context(), merchantID, batchNo)
	if err != nil {
		_ = c.Error(err)
		return
	}
	errs.OK(c, resp)
}

// BillStores GET /v1/merchant/split/bills/:batch_no/stores（批次下的门店汇总）。
func (h *Handler) BillStores(c *gin.Context) {
	merchantID := c.GetUint64("merchant_id")
	if merchantID == 0 {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, "merchant_id required", 200))
		return
	}
	batchNo := c.Param("batch_no")
	if batchNo == "" {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, "batch_no required", 200))
		return
	}
	resp, err := h.svc.BillStoreSummary(c.Request.Context(), merchantID, batchNo)
	if err != nil {
		_ = c.Error(err)
		return
	}
	errs.OK(c, resp)
}

// BillStoreOrders GET /v1/merchant/split/bills/:batch_no/stores/:store_id/orders（批次内某门店订单明细）。
func (h *Handler) BillStoreOrders(c *gin.Context) {
	merchantID := c.GetUint64("merchant_id")
	if merchantID == 0 {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, "merchant_id required", 200))
		return
	}
	batchNo := c.Param("batch_no")
	storeID, err := strconv.ParseUint(c.Param("store_id"), 10, 64)
	if batchNo == "" || err != nil || storeID == 0 {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, "batch_no/store_id invalid", 200))
		return
	}
	resp, err := h.svc.BillStoreOrders(c.Request.Context(), merchantID, batchNo, storeID)
	if err != nil {
		_ = c.Error(err)
		return
	}
	errs.OK(c, resp)
}

// ApproveBill POST /v1/merchant/split/bills/:batch_no/approve。
func (h *Handler) ApproveBill(c *gin.Context) {
	merchantID := c.GetUint64("merchant_id")
	if merchantID == 0 {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, "merchant_id required", 200))
		return
	}
	batchNo := c.Param("batch_no")
	resp, err := h.svc.ApproveBill(c.Request.Context(), merchantID, batchNo)
	if err != nil {
		_ = c.Error(err)
		return
	}
	errs.OK(c, resp)
}

// RejectBill POST /v1/merchant/split/bills/:batch_no/reject。
func (h *Handler) RejectBill(c *gin.Context) {
	merchantID := c.GetUint64("merchant_id")
	if merchantID == 0 {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, "merchant_id required", 200))
		return
	}
	batchNo := c.Param("batch_no")
	resp, err := h.svc.RejectBill(c.Request.Context(), merchantID, batchNo)
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

// ListExecutions GET /v1/merchant/split/executions（分页，商户隔离，支持状态/时间/规则过滤）。
func (h *Handler) ListExecutions(c *gin.Context) {
	merchantID := c.GetUint64("merchant_id")
	if merchantID == 0 {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, "merchant_id required", 200))
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "20"))
	ruleID, _ := strconv.ParseUint(c.DefaultQuery("rule_id", "0"), 10, 64)
	f := service.ExecutionFilter{
		Status: c.Query("status"),
		Start:  c.Query("start"),
		End:    c.Query("end"),
		RuleID: ruleID,
	}
	resp, err := h.svc.ListExecutions(c.Request.Context(), merchantID, page, size, f)
	if err != nil {
		_ = c.Error(err)
		return
	}
	errs.OK(c, resp)
}

// Preview POST /v1/merchant/split/preview（分账试算，不落库）。
func (h *Handler) Preview(c *gin.Context) {
	merchantID := c.GetUint64("merchant_id")
	if merchantID == 0 {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, "merchant_id required", 200))
		return
	}
	var req service.PreviewRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, err.Error(), 200))
		return
	}
	resp, err := h.svc.Preview(c.Request.Context(), merchantID, &req)
	if err != nil {
		_ = c.Error(err)
		return
	}
	errs.OK(c, resp)
}

// RetryExecution POST /v1/merchant/split/executions/:order_no/retry（重试失败/部分失败分账）。
func (h *Handler) RetryExecution(c *gin.Context) {
	merchantID := c.GetUint64("merchant_id")
	if merchantID == 0 {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, "merchant_id required", 200))
		return
	}
	no := c.Param("order_no")
	if no == "" {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, "order_no required", 200))
		return
	}
	resp, err := h.svc.RetryExecution(c.Request.Context(), merchantID, no)
	if err != nil {
		_ = c.Error(err)
		return
	}
	errs.OK(c, resp)
}

// GetExecutionDetail GET /v1/merchant/split/executions/:order_no（商户隔离）。
func (h *Handler) GetExecutionDetail(c *gin.Context) {
	merchantID := c.GetUint64("merchant_id")
	if merchantID == 0 {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, "merchant_id required", 200))
		return
	}
	no := c.Param("order_no")
	if no == "" {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, "order_no required", 200))
		return
	}
	rows, err := h.svc.GetExecutionDetail(c.Request.Context(), merchantID, no)
	if err != nil {
		_ = c.Error(err)
		return
	}
	errs.OK(c, gin.H{"order_no": no, "items": rows})
}

// ============ 差错中心 ============

// parsePageQuery 解析分页参数（默认 page=1, size=20，size 上限 100）。
func parsePageQuery(c *gin.Context) (int, int) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "20"))
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}
	return page, size
}

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

// ReopenExecution POST /v1/merchant/split/executions/:order_no/reopen（死单复位重开）。
func (h *Handler) ReopenExecution(c *gin.Context) {
	merchantID := c.GetUint64("merchant_id")
	if merchantID == 0 {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, "merchant_id required", 200))
		return
	}
	no := c.Param("order_no")
	if no == "" {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, "order_no required", 200))
		return
	}
	if err := h.svc.ReopenExecution(c.Request.Context(), merchantID, no); err != nil {
		_ = c.Error(err)
		return
	}
	errs.OK(c, gin.H{"order_no": no, "reopened": true})
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
	startStr, endStr := c.Query("start_date"), c.Query("end_date")
	start, err1 := time.ParseInLocation("2006-01-02", startStr, time.Local)
	end, err2 := time.ParseInLocation("2006-01-02", endStr, time.Local)
	if err1 != nil || err2 != nil {
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