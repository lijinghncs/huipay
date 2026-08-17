package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/huipay/huipay-backend/infra/errs"
	"github.com/huipay/huipay-backend/internal/split/service"
)

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
	page, size := parsePageQuery(c)
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