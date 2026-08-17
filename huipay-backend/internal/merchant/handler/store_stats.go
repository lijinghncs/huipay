// 包 handler 暴露商户自助门店日报统计 HTTP 接口（读中间件 merchant_id，按当前商户过滤）。
package handler

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/huipay/huipay-backend/infra/errs"
	statsservice "github.com/huipay/huipay-backend/internal/stats/service"
)

// StoreStatsHandler 商户门店日报统计 Handler。
type StoreStatsHandler struct {
	svc    *statsservice.Service
	logger *zap.Logger
}

// NewStoreStatsHandler 构造 StoreStatsHandler。
func NewStoreStatsHandler(svc *statsservice.Service, logger *zap.Logger) *StoreStatsHandler {
	return &StoreStatsHandler{svc: svc, logger: logger}
}

func (h *StoreStatsHandler) merchantID(c *gin.Context) (uint64, bool) {
	id, ok := c.Get("merchant_id")
	if !ok {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, "missing merchant id", 401))
		return 0, false
	}
	return id.(uint64), true
}

func (h *StoreStatsHandler) parseRange(c *gin.Context) (time.Time, time.Time, bool) {
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
	end = end.AddDate(0, 0, 1) // 含 end 当日
	return start, end, true
}

// ListStats GET /v1/merchant/store-stats 门店日报列表（当前商户）。
func (h *StoreStatsHandler) ListStats(c *gin.Context) {
	mid, ok := h.merchantID(c)
	if !ok {
		return
	}
	start, end, ok := h.parseRange(c)
	if !ok {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, "start_date and end_date required (YYYY-MM-DD)", 200))
		return
	}
	var storeID *uint64
	if v := c.Query("store_id"); v != "" {
		if n, err := strconv.ParseUint(v, 10, 64); err == nil && n > 0 {
			storeID = &n
		}
	}
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
	items, total, err := h.svc.ListStats(c.Request.Context(), mid, storeID, start, end, page, size)
	if err != nil {
		_ = c.Error(err)
		return
	}
	errs.OK(c, gin.H{"items": items, "total": total})
}

// Summary GET /v1/merchant/store-stats/summary 多日范围门店汇总（当前商户）。
func (h *StoreStatsHandler) Summary(c *gin.Context) {
	mid, ok := h.merchantID(c)
	if !ok {
		return
	}
	start, end, ok := h.parseRange(c)
	if !ok {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, "start_date and end_date required (YYYY-MM-DD)", 200))
		return
	}
	var storeID *uint64
	if v := c.Query("store_id"); v != "" {
		if n, err := strconv.ParseUint(v, 10, 64); err == nil && n > 0 {
			storeID = &n
		}
	}
	summary, rows, err := h.svc.Summary(c.Request.Context(), mid, storeID, start, end)
	if err != nil {
		_ = c.Error(err)
		return
	}
	errs.OK(c, gin.H{"summary": summary, "items": rows})
}

// GetDailyStats GET /v1/merchant/store-stats/stores/:id/daily 单门店按日明细（当前商户）。
func (h *StoreStatsHandler) GetDailyStats(c *gin.Context) {
	mid, ok := h.merchantID(c)
	if !ok {
		return
	}
	storeID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, "store id invalid", 200))
		return
	}
	start, end, ok := h.parseRange(c)
	if !ok {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, "start_date and end_date required (YYYY-MM-DD)", 200))
		return
	}
	// 校验门店归属当前商户，避免越权
	items, err := h.svc.GetStoreDailyStats(c.Request.Context(), mid, storeID, start, end)
	if err != nil {
		_ = c.Error(err)
		return
	}
	errs.OK(c, items)
}