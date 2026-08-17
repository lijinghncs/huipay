// 包 handler 暴露管理后台调度任务监测与门店报表 HTTP 接口。
package handler

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/huipay/huipay-backend/infra/errs"
	adminservice "github.com/huipay/huipay-backend/internal/admin/service"
)

// SchedulerHandler 调度任务监测 Handler。
type SchedulerHandler struct {
	svc    *adminservice.SchedulerService
	logger *zap.Logger
}

// NewSchedulerHandler 构造 SchedulerHandler。
func NewSchedulerHandler(svc *adminservice.SchedulerService, logger *zap.Logger) *SchedulerHandler {
	return &SchedulerHandler{svc: svc, logger: logger}
}

// ListTasks GET /v1/admin/scheduler/tasks 已注册任务列表。
func (h *SchedulerHandler) ListTasks(c *gin.Context) {
	list, err := h.svc.ListTasks(c.Request.Context())
	if err != nil {
		_ = c.Error(err)
		return
	}
	errs.OK(c, list)
}

// ListRuns GET /v1/admin/scheduler/runs 运行日志列表。
func (h *SchedulerHandler) ListRuns(c *gin.Context) {
	page := parsePage(c.Query("page"))
	size := parseSize(c.Query("page_size"))
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

// GetRun GET /v1/admin/scheduler/runs/:id 单次运行详情。
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

// TriggerTask POST /v1/admin/scheduler/tasks/:name/run 手工触发任务。
func (h *SchedulerHandler) TriggerTask(c *gin.Context) {
	name := c.Param("name")
	if name == "" {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, "task name required", 200))
		return
	}
	supported, err := h.svc.TriggerTask(c.Request.Context(), name, nil)
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

func parsePage(v string) int {
	if v == "" {
		return 1
	}
	if n, err := strconv.Atoi(v); err == nil && n >= 1 {
		return n
	}
	return 1
}

func parseSize(v string) int {
	if v == "" {
		return 20
	}
	if n, err := strconv.Atoi(v); err == nil && n >= 1 && n <= 200 {
		return n
	}
	return 20
}