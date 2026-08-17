package handler

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/huipay/huipay-backend/infra/errs"
	statsservice "github.com/huipay/huipay-backend/internal/stats/service"
)

// StoreStatsHandler 门店日报报表 Handler（admin）。
type StoreStatsHandler struct {
	svc    *statsservice.Service
	logger *zap.Logger
}

// NewStoreStatsHandler 构造 StoreStatsHandler。
func NewStoreStatsHandler(svc *statsservice.Service, logger *zap.Logger) *StoreStatsHandler {
	return &StoreStatsHandler{svc: svc, logger: logger}
}

// parseRange 解析 start_date / end_date（YYYY-MM-DD），end 含当日（取下一天为开区间）。
func parseRange(c *gin.Context) (time.Time, time.Time, bool) {
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

func parseUintQuery(c *gin.Context, key string) uint64 {
	v := c.Query(key)
	if v == "" {
		return 0
	}
	n, _ := strconv.ParseUint(v, 10, 64)
	return n
}

func parseUintParam(c *gin.Context, key string) (uint64, error) {
	return strconv.ParseUint(c.Param(key), 10, 64)
}

// ListStoreStats GET /v1/admin/store-stats 门店日报列表。
func (h *StoreStatsHandler) ListStoreStats(c *gin.Context) {
	start, end, ok := parseRange(c)
	if !ok {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, "start_date and end_date required (YYYY-MM-DD)", 200))
		return
	}
	merchantID := parseUintQuery(c, "merchant_id")
	var storeID *uint64
	if v := parseUintQuery(c, "store_id"); v > 0 {
		storeID = &v
	}
	page := parsePage(c.Query("page"))
	size := parseSize(c.Query("page_size"))
	items, total, err := h.svc.ListStats(c.Request.Context(), merchantID, storeID, start, end, page, size)
	if err != nil {
		_ = c.Error(err)
		return
	}
	errs.OK(c, gin.H{"items": items, "total": total})
}

// StoreStatsSummary GET /v1/admin/store-stats/summary 多日范围门店汇总。
func (h *StoreStatsHandler) StoreStatsSummary(c *gin.Context) {
	start, end, ok := parseRange(c)
	if !ok {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, "start_date and end_date required (YYYY-MM-DD)", 200))
		return
	}
	merchantID := parseUintQuery(c, "merchant_id")
	var storeID *uint64
	if v := parseUintQuery(c, "store_id"); v > 0 {
		storeID = &v
	}
	summary, rows, err := h.svc.Summary(c.Request.Context(), merchantID, storeID, start, end)
	if err != nil {
		_ = c.Error(err)
		return
	}
	errs.OK(c, gin.H{"summary": summary, "items": rows})
}

// GetStoreDailyStats GET /v1/admin/stores/:id/daily-stats 单门店按日明细。
func (h *StoreStatsHandler) GetStoreDailyStats(c *gin.Context) {
	storeID, err := parseUintParam(c, "id")
	if err != nil {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, "store id invalid", 200))
		return
	}
	start, end, ok := parseRange(c)
	if !ok {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, "start_date and end_date required (YYYY-MM-DD)", 200))
		return
	}
	items, err := h.svc.GetStoreDailyStats(c.Request.Context(), 0, storeID, start, end)
	if err != nil {
		_ = c.Error(err)
		return
	}
	errs.OK(c, items)
}

// Backfill POST /v1/admin/store-stats/backfill 补跑历史门店日报（admin 运维用）。
func (h *StoreStatsHandler) Backfill(c *gin.Context) {
	start, end, ok := parseRange(c)
	if !ok {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, "start_date and end_date required (YYYY-MM-DD)", 200))
		return
	}
	if end.Before(start) {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, "end_date must be >= start_date", 200))
		return
	}
	rows, err := h.svc.Backfill(c.Request.Context(), start, end)
	if err != nil {
		_ = c.Error(err)
		return
	}
	errs.OK(c, gin.H{"rows": rows})
}