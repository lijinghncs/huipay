package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/huipay/huipay-backend/infra/errs"
	"github.com/huipay/huipay-backend/internal/split/service"
)

// Execute POST /v1/split/execute。
func (h *Handler) Execute(c *gin.Context) {
	var req service.ExecuteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, err.Error(), 200))
		return
	}
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
	page, size := parsePageQuery(c)
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