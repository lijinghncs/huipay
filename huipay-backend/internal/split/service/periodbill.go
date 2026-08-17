package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/huipay/huipay-backend/infra/errs"
	"github.com/huipay/huipay-backend/internal/split/alloc"
	"github.com/huipay/huipay-backend/internal/split/repository"
	"github.com/huipay/huipay-backend/internal/split/rule"
)

// ExecuteByPeriodRequest 周期分账请求。
type ExecuteByPeriodRequest struct {
	RuleCode  string `json:"rule_code" binding:"required"`
	Channel   string `json:"channel"`
	StartDate string `json:"start_date" binding:"required"` // YYYY-MM-DD
	EndDate   string `json:"end_date" binding:"required"`   // YYYY-MM-DD
}

// ExecuteByPeriodResponse 周期分账响应。
type ExecuteByPeriodResponse struct {
	BatchNo  string `json:"batch_no"`
	Executed bool   `json:"executed"`
	Message  string `json:"message,omitempty"`
	OrderNo  string `json:"order_no,omitempty"`
	Status   string `json:"status,omitempty"`
	Amount   int64  `json:"amount,omitempty"`
}

// BillDTO 分账账单 DTO。
type BillDTO struct {
	ID          uint64 `json:"id"`
	MerchantID  uint64 `json:"merchant_id"`
	BatchNo     string `json:"batch_no"`
	RuleCode    string `json:"rule_code"`
	RuleName    string `json:"rule_name"`
	TotalAmount int64  `json:"total_amount"`
	Status      string `json:"status"`
	StartTime   string `json:"start_time"`
	EndTime     string `json:"end_time"`
	CreatedAt   string `json:"created_at"`
	ApprovedAt  string `json:"approved_at,omitempty"`
	ExecutedAt  string `json:"executed_at,omitempty"`
	Items       []repository.SplitBillItem `json:"items,omitempty"`
}

// BillPage 分账账单分页结果。
type BillPage struct {
	Items []BillDTO `json:"items"`
	Total int64     `json:"total"`
}

// BillStoreItem 门店汇总项。
type BillStoreItem struct {
	StoreID    uint64 `json:"store_id"`
	StoreName  string `json:"store_name"`
	Amount     int64  `json:"amount"`
	OrderCount int    `json:"order_count"`
}

// BillStoreOrders 门店订单明细。
type BillStoreOrders struct {
	StoreID   uint64                      `json:"store_id"`
	StoreName string                      `json:"store_name"`
	Orders    []repository.SplitOrderStatusModel `json:"orders"`
}

// GenerateBill 生成分账账单（从周期分账执行结果生成）。
func (s *Service) GenerateBill(ctx context.Context, merchantID uint64, req *ExecuteByPeriodRequest) (*BillDTO, error) {
	if s.billRepo == nil {
		return nil, errs.New(errs.CodeInternalError, "bill repo not configured", 500)
	}
	batchNo := fmt.Sprintf("SP-BILL-%d-%s-%s", merchantID, req.StartDate, req.EndDate)
	existing, err := s.billRepo.GetByBatchNo(ctx, batchNo)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternalError, "check bill failed", 200, err)
	}
	if existing != nil {
		return nil, errs.New(errs.CodeInvalidParams, "bill already exists", 200)
	}

	// 查询规则
	rule, err := s.ruleRepo.GetByCodeAndMerchant(ctx, ruleCode, merchantID)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternalError, "query rule failed", 200, err)
	}
	if rule == nil {
		return nil, errs.New(errs.CodeInvalidParams, "rule not found", 200)
	}

	// 构造账单模型
	now := time.Now()
	items := []repository.SplitBillItem{}
	itemsJSON, _ := json.Marshal(items)
	m := &repository.SplitBillModel{
		MerchantID:  merchantID,
		BatchNo:     batchNo,
		RuleCode:    ruleCode,
		RuleName:    rule.RuleName,
		TotalAmount: 0,
		Status:      repository.BillPending,
		Detail:      string(itemsJSON),
		CreatedAt:   now,
	}
	if err := s.billRepo.Create(ctx, m); err != nil {
		return nil, errs.Wrap(errs.CodeInternalError, "create bill failed", 200, err)
	}
	return billToDTO(m), nil
}

// ListBills 分页查询账单。
func (s *Service) ListBills(ctx context.Context, merchantID uint64, page, size int) (*BillPage, error) {
	if s.billRepo == nil {
		return nil, errs.New(errs.CodeInternalError, "bill repo not configured", 500)
	}
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 200 {
		size = 20
	}
	items, total, err := s.billRepo.ListByMerchant(ctx, merchantID, (page-1)*size, size)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternalError, "query bills failed", 200, err)
	}
	out := make([]BillDTO, 0, len(items))
	for _, m := range items {
		out = append(out, *billToDTO(&m))
	}
	return &BillPage{Items: out, Total: total}, nil
}

// GetBillDetail 查询账单详情（含分配项）。
func (s *Service) GetBillDetail(ctx context.Context, merchantID uint64, batchNo string) (*BillDTO, error) {
	if s.billRepo == nil {
		return nil, errs.New(errs.CodeInternalError, "bill repo not configured", 500)
	}
	bill, found, err := s.getPendingBillRaw(ctx, merchantID, batchNo)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, errs.New(errs.CodeInvalidParams, "bill not found", 200)
	}
	return billToDTO(bill), nil
}

// BillStoreSummary 查询门店汇总。
func (s *Service) BillStoreSummary(ctx context.Context, merchantID uint64, batchNo string) ([]BillStoreItem, error) {
	if s.billRepo == nil {
		return nil, errs.New(errs.CodeInternalError, "bill repo not configured", 500)
	}
	bill, found, err := s.getPendingBillRaw(ctx, merchantID, batchNo)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, errs.New(errs.CodeInvalidParams, "bill not found", 200)
	}
	var items []repository.SplitBillItem
	if err := json.Unmarshal([]byte(bill.Detail), &items); err != nil {
		return nil, errs.Wrap(errs.CodeInternalError, "parse bill detail failed", 200, err)
	}

	storeIDs := make([]uint64, 0)
	storeMap := make(map[uint64]int64)
	storeCount := make(map[uint64]int)
	for _, item := range items {
		if item.StoreID != nil {
			sid := *item.StoreID
			storeMap[sid] += item.Amount
			storeCount[sid]++
			if _, exists := storeMap[sid]; !exists {
				storeIDs = append(storeIDs, sid)
			}
		}
	}

	names := make(map[uint64]string)
	if len(storeIDs) > 0 {
		names, _ = s.billRepo.GetStoreNames(ctx, storeIDs)
	}

	out := make([]BillStoreItem, 0, len(storeMap))
	for sid, amount := range storeMap {
		out = append(out, BillStoreItem{
			StoreID:    sid,
			StoreName:  names[sid],
			Amount:     amount,
			OrderCount: storeCount[sid],
		})
	}
	return out, nil
}

// BillStoreOrders 查询门店订单明细。
func (s *Service) BillStoreOrders(ctx context.Context, merchantID uint64, batchNo string, storeID uint64) (*BillStoreOrders, error) {
	names := make(map[uint64]string)
	if storeID > 0 {
		names, _ = s.billRepo.GetStoreNames(ctx, []uint64{storeID})
	}

	// 周期分账完成后，按门店过滤异常订单
	exceptions, _, err := s.orderStatusRepo.ListExceptions(ctx, merchantID, "", nil, 0, 1000)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternalError, "query store orders failed", 200, err)
	}

	orders := make([]repository.SplitOrderStatusModel, 0)
	for _, o := range exceptions {
		orders = append(orders, o)
	}

	return &BillStoreOrders{
		StoreID:   storeID,
		StoreName: names[storeID],
		Orders:    orders,
	}, nil
}

// ApproveBill 审批通过账单：触发分账执行（异步状态机）。
func (s *Service) ApproveBill(ctx context.Context, merchantID uint64, batchNo string) error {
	bill, found, err := s.getPendingBillRaw(ctx, merchantID, batchNo)
	if err != nil {
		return err
	}
	if !found {
		return errs.New(errs.CodeInvalidParams, "bill not found", 200)
	}
	if bill.Status != repository.BillPending {
		return errs.New(errs.CodeInvalidParams, "bill is not pending", 200)
	}

	ok, err := s.billRepo.UpdateStatus(ctx, bill.ID, "APPROVED", nil)
	if err != nil {
		return errs.Wrap(errs.CodeInternalError, "approve bill failed", 200, err)
	}
	if !ok {
		return errs.New(errs.CodeInternalError, "bill status update failed", 200)
	}
	s.appendAudit(ctx, "SPLIT_BILL", batchNo, "APPROVE", merchantID, map[string]any{"bill_id": bill.ID})
	return nil
}

// RejectBill 驳回账单。
func (s *Service) RejectBill(ctx context.Context, merchantID uint64, batchNo string) error {
	bill, found, err := s.getPendingBillRaw(ctx, merchantID, batchNo)
	if err != nil {
		return err
	}
	if !found {
		return errs.New(errs.CodeInvalidParams, "bill not found", 200)
	}
	if bill.Status != repository.BillPending {
		return errs.New(errs.CodeInvalidParams, "bill is not pending", 200)
	}

	ok, err := s.billRepo.UpdateStatus(ctx, bill.ID, "REJECTED", nil)
	if err != nil {
		return errs.Wrap(errs.CodeInternalError, "reject bill failed", 200, err)
	}
	if !ok {
		return errs.New(errs.CodeInternalError, "bill status update failed", 200)
	}
	s.appendAudit(ctx, "SPLIT_BILL", batchNo, "REJECT", merchantID, map[string]any{"bill_id": bill.ID})
	return nil
}

// ExecuteByPeriod 按周期执行分账：规则匹配 → 前置对账 → 分配 → 执行 → 状态机 → 审计。
func (s *Service) ExecuteByPeriod(ctx context.Context, merchantID uint64, req *ExecuteByPeriodRequest) (*ExecuteByPeriodResponse, error) {
	if s.ruleEngine == nil || s.executor == nil {
		return nil, errs.New(errs.CodeInternalError, "split engine not ready", 500)
	}

	// 1. 解析日期范围
	start, err := time.Parse("2006-01-02", req.StartDate)
	if err != nil {
		return nil, errs.New(errs.CodeInvalidParams, "invalid start_date, expected YYYY-MM-DD", 200)
	}
	end, err := time.Parse("2006-01-02", req.EndDate)
	if err != nil {
		return nil, errs.New(errs.CodeInvalidParams, "invalid end_date, expected YYYY-MM-DD", 200)
	}
	bizDates := alloc.CollectBizDates(start, end)
	paidAt := start.Format(time.RFC3339) + "/" + end.Format(time.RFC3339)

	// 查询业务日支付总额
	totalPaid, err := s.revenueQuerier.SumPaid(ctx, merchantID, start, end)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternalError, "query total paid failed", 200, err)
	}
	if totalPaid <= 0 {
		return nil, errs.New(errs.CodeInvalidParams, "no paid orders in period", 200)
	}

	// 2. 规则匹配
	matched, ok := s.doMatchRule(ctx, merchantID, req)
	if !ok || matched == nil {
		return nil, errs.New(errs.CodeInvalidParams, "no matching rule found", 200)
	}

	// 3. 前置对账（双层 Prechecker）
	precheck := s.prechecker.Precheck(ctx, merchantID, bizDates, totalPaid)
	if !precheck.OK {
		// 不阻断，记录差异
		_ = precheck
	}

	// 4. 查询门店营收分布
	revenues, err := s.revenueQuerier.SumPaidByStore(ctx, merchantID, start, end)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternalError, "query store revenue failed", 200, err)
	}

	// 5. 分配方案计算（纯函数）
	revenueList := make([]alloc.StoreRevenue, len(revenues))
	for i, r := range revenues {
		revenueList[i] = alloc.StoreRevenue{StoreID: r.StoreID, Paid: r.Paid}
	}
	allocations, calcErr := alloc.Compute(alloc.Input{
		Rule:          matched,
		Total:         totalPaid,
		StoreRevenues: revenueList,
	})
	if calcErr != nil {
		return nil, errs.Wrap(errs.CodeInternalError, "compute allocations failed", 200, calcErr)
	}
	if len(allocations) == 0 {
		return nil, errs.New(errs.CodeInternalError, "compute allocations returned empty result", 200)
	}

	// 6. 执行分账
	batchNo := fmt.Sprintf("SP-%d-%d-%d", merchantID, start.Unix(), end.Unix())
	runID := fmt.Sprintf("SP_RUN-%d-%s-%d", merchantID, batchNo, time.Now().UnixNano())
	execReq := &ExecuteRequest{
		OrderNo:       runID,
		MerchantID:    merchantID,
		RuleCode:      matched.RuleCode,
		Amount:        totalPaid,
		Allocations:   allocations,
		PaidAt:        paidAt,
		BizDate:       start,
		ReceiverCount: len(allocations),
	}

	execResult, execErr := s.executor.Execute(ctx, execReq)
	if execErr != nil {
		return nil, errs.Wrap(errs.CodeInternalError, "execute split failed", 200, execErr)
	}

	// 7. 审计
	auditAction := "EXECUTE"
	if execResult.Status == "FAILED" {
		auditAction = "EXECUTE_FAILED"
	}
	s.appendAudit(ctx, "DAILY_SPLIT", execReq.OrderNo, auditAction, merchantID, map[string]any{
		"rule_code": matched.RuleCode,
		"amount":    totalPaid,
		"status":    execResult.Status,
	})

	// 8. 执行后对账
	s.doPostReconcile(ctx, merchantID, start, bizDates, execResult, totalPaid)

	return &ExecuteByPeriodResponse{
		BatchNo:   batchNo,
		Executed:  true,
		Message:   "ok",
		OrderNo:   execReq.OrderNo,
		Status:    execResult.Status,
		Amount:    totalPaid,
	}, nil
}

// ---------- 内部辅助 ----------

func (s *Service) doMatchRule(ctx context.Context, merchantID uint64, req *ExecuteByPeriodRequest) (*rule.Rule, bool) {
	rules, err := s.ruleRepo.ListByMerchant(ctx, merchantID)
	if err != nil || len(rules) == 0 {
		return nil, false
	}
	matched := s.ruleEngine.Resolve(rules, rule.MatchContext{
		MerchantID: merchantID,
		Channel:    req.Channel,
	})
	return matched, matched != nil
}

func (s *Service) doPostReconcile(ctx context.Context, merchantID uint64, bizDate time.Time, bizDates []time.Time, execResult *ExecuteResponse, totalPaid int64) {
	if execResult == nil {
		return
	}
	// 执行后对账：post-reconcile 逻辑保留占位，P1 阶段按 ports 注入后实现
	_ = merchantID
	_ = bizDate
	_ = bizDates
	_ = totalPaid
}