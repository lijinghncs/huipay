// 包 handler 暴露商户自助定时任务监测 HTTP 接口（只读：任务列表 + 运行日志）。
package handler

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/huipay/huipay-backend/infra/errs"
	adminservice "github.com/huipay/huipay-backend/internal/admin/service"
)

// SchedulerHandler 商户定时任务监测 Handler。
type SchedulerHandler struct {
	svc    *adminservice.SchedulerService
	logger *zap.Logger
}

// NewSchedulerHandler 构造 SchedulerHandler。
func NewSchedulerHandler(svc *adminservice.SchedulerService, logger *zap.Logger) *SchedulerHandler {
	return &SchedulerHandler{svc: svc, logger: logger}
}

// ListTasks GET /v1/merchant/scheduler/tasks 已注册任务列表（只读）。
func (h *SchedulerHandler) ListTasks(c *gin.Context) {
	list, err := h.svc.ListTasks(c.Request.Context())
	if err != nil {
		_ = c.Error(err)
		return
	}
	errs.OK(c, list)
}

// ListRuns GET /v1/merchant/scheduler/runs 运行日志列表（只读）。
func (h *SchedulerHandler) ListRuns(c *gin.Context) {
	page := 1
	if v := c.Query("page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 1 {
			page = n
		}
	}
	size := 20
	if v := c.Query("page_size"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 1 && n <= 200 {
			size = n
		}
	}
	f := adminservice.RunFilter{
		Name:     c.Query("name"),
		Status:   c.Query("status"),
		Page:     page,
		PageSize: size,
	}
	if v := c.Query("start"); v != "" {
		if t, err := time.ParseInLocation("2006-01-02 15:04:05", v, time.Local); err == nil {
			f.Start = &t
		}
	}
	if v := c.Query("end"); v != "" {
		if t, err := time.ParseInLocation("2006-01-02 15:04:05", v, time.Local); err == nil {
			f.End = &t
		}
	}
	list, err := h.svc.ListRuns(c.Request.Context(), f)
	if err != nil {
		_ = c.Error(err)
		return
	}
	errs.OK(c, list)
}

// GetRun GET /v1/merchant/scheduler/runs/:id 单次运行详情（只读）。
func (h *SchedulerHandler) GetRun(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, "id invalid", 200))
		return
	}
	run, err := h.svc.GetRun(c.Request.Context(), id)
	if err != nil {
		_ = c.Error(err)
		return
	}
	if run == nil {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, "run not found", 200))
		return
	}
	errs.OK(c, run)
}

// TriggerTask POST /v1/merchant/scheduler/tasks/:name/run 手动执行任务。
// 可选查询参数 biz_date（YYYY-MM-DD）作为业务日期传入；缺省使用任务默认日期。
func (h *SchedulerHandler) TriggerTask(c *gin.Context) {
	name := c.Param("name")
	if name == "" {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, "task name required", 200))
		return
	}
	var bizDate *time.Time
	if v := c.Query("biz_date"); v != "" {
		t, err := time.ParseInLocation("2006-01-02", v, time.Local)
		if err != nil {
			errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, "biz_date invalid (YYYY-MM-DD)", 200))
			return
		}
		bizDate = &t
	}
	supported, err := h.svc.TriggerTask(c.Request.Context(), name, bizDate)
	if err != nil {
		_ = c.Error(err)
		return
	}
	if !supported {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, "task does not support manual trigger", 200))
		return
	}
	errs.OK(c, gin.H{"triggered": true})
}