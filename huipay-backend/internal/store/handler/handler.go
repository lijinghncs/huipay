// 包 handler 暴露门店 HTTP 接口。
package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/huipay/huipay-backend/infra/errs"
	"github.com/huipay/huipay-backend/internal/store/service"
)

// Handler 门店 Handler。
type Handler struct {
	svc    *service.Service
	logger *zap.Logger
}

// New 构造 Handler。
func New(svc *service.Service, logger *zap.Logger) *Handler {
	return &Handler{svc: svc, logger: logger}
}

// merchantID 从中间件取当前登录商户号（X-Merchant-Id）。
func (h *Handler) merchantID(c *gin.Context) (uint64, bool) {
	id := c.GetUint64("merchant_id")
	if id == 0 {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, "merchant_id required", 200))
		return 0, false
	}
	return id, true
}

// createReq 创建门店请求体。
type createReq struct {
	Name         string         `json:"name"`
	StoreType    string         `json:"store_type"`
	ContactPhone string         `json:"contact_phone"`
	Region       string         `json:"region"`
	Address      string         `json:"address"`
	Longitude    *float64       `json:"longitude"`
	Latitude     *float64       `json:"latitude"`
	Metadata     map[string]any `json:"metadata"`
}

// Create POST /v1/merchant/stores 创建门店。
func (h *Handler) Create(c *gin.Context) {
	mid, ok := h.merchantID(c)
	if !ok {
		return
	}
	var req createReq
	if err := c.ShouldBindJSON(&req); err != nil {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, "invalid request body", 200))
		return
	}
	store, err := h.svc.Create(c.Request.Context(), &service.CreateRequest{
		MerchantID:   mid,
		Name:         req.Name,
		StoreType:    req.StoreType,
		ContactPhone: req.ContactPhone,
		Region:       req.Region,
		Address:      req.Address,
		Longitude:    req.Longitude,
		Latitude:     req.Latitude,
		Metadata:     req.Metadata,
	})
	if err != nil {
		_ = c.Error(err)
		return
	}
	errs.OK(c, store)
}

// List GET /v1/merchant/stores 门店列表。
func (h *Handler) List(c *gin.Context) {
	mid, ok := h.merchantID(c)
	if !ok {
		return
	}
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
	list, err := h.svc.List(c.Request.Context(), mid, page, size, c.Query("keyword"), status)
	if err != nil {
		_ = c.Error(err)
		return
	}
	errs.OK(c, list)
}

// Stats GET /v1/merchant/stores/stats 门店统计。
func (h *Handler) Stats(c *gin.Context) {
	mid, ok := h.merchantID(c)
	if !ok {
		return
	}
	stats, err := h.svc.Stats(c.Request.Context(), mid)
	if err != nil {
		_ = c.Error(err)
		return
	}
	errs.OK(c, stats)
}

// Get GET /v1/merchant/stores/:id 门店详情。
func (h *Handler) Get(c *gin.Context) {
	mid, ok := h.merchantID(c)
	if !ok {
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, "id invalid", 200))
		return
	}
	d, err := h.svc.Get(c.Request.Context(), id, mid)
	if err != nil {
		_ = c.Error(err)
		return
	}
	errs.OK(c, d)
}

// Update PUT /v1/merchant/stores/:id 更新门店。
func (h *Handler) Update(c *gin.Context) {
	mid, ok := h.merchantID(c)
	if !ok {
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, "id invalid", 200))
		return
	}
	var req createReq
	if err := c.ShouldBindJSON(&req); err != nil {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, "invalid request body", 200))
		return
	}
	store, err := h.svc.Update(c.Request.Context(), id, mid, &service.UpdateRequest{
		Name:         req.Name,
		StoreType:    req.StoreType,
		ContactPhone: req.ContactPhone,
		Region:       req.Region,
		Address:      req.Address,
		Longitude:    req.Longitude,
		Latitude:     req.Latitude,
		Metadata:     req.Metadata,
	})
	if err != nil {
		_ = c.Error(err)
		return
	}
	errs.OK(c, store)
}

// statusReq 启停请求体。
type statusReq struct {
	Status int `json:"status"`
}

// SetStatus POST /v1/merchant/stores/:id/status 启停门店。
func (h *Handler) SetStatus(c *gin.Context) {
	mid, ok := h.merchantID(c)
	if !ok {
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, "id invalid", 200))
		return
	}
	var req statusReq
	if err := c.ShouldBindJSON(&req); err != nil {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, "invalid request body", 200))
		return
	}
	store, err := h.svc.SetStatus(c.Request.Context(), id, mid, req.Status)
	if err != nil {
		_ = c.Error(err)
		return
	}
	errs.OK(c, store)
}

// Delete DELETE /v1/merchant/stores/:id 删除门店。
func (h *Handler) Delete(c *gin.Context) {
	mid, ok := h.merchantID(c)
	if !ok {
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, "id invalid", 200))
		return
	}
	if err := h.svc.Delete(c.Request.Context(), id, mid); err != nil {
		_ = c.Error(err)
		return
	}
	errs.OK(c, gin.H{"id": id, "deleted": true})
}