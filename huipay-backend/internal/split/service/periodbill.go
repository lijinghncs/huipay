package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/huipay/huipay-backend/infra/errs"
	"github.com/huipay/huipay-backend/infra/prom"
	"github.com/huipay/huipay-backend/internal/domain/vo"
	"github.com/huipay/huipay-backend/internal/split/alloc"
	"github.com/huipay/huipay-backend/internal/split/executor"
	"github.com/huipay/huipay-backend/internal/split/repository"
	"github.com/huipay/huipay-backend/internal/split/rule"
)

// ExecuteByPeriodRequest 时段分账请求。
type ExecuteByPeriodRequest struct {
	MerchantID uint64   `json:"merchant_id"`
	RuleCode   string   `json:"rule_code"`              // 指定规则（可选，为空则自动匹配）
	RuleIDs    []uint64 `json:"rule_ids"`               // 指定规则ID列表（可选，铁路模式允许一次执行多规则）
	StoreIDs   []uint64 `json:"store_ids"`              // 指定门店（可选，为空则全门店）
	Start      string   `json:"start" binding:"required"` // RFC3339
	End        string   `json:"end" binding:"required"`   // RFC3339
	Channel    string   `json:"channel"`
	TraceID    string   `json:"-"`
}

// ExecuteByPeriodResponse 时段分账响应。
type ExecuteByPeriodResponse struct {
	BizDate string `json:"biz_date"` // 业务日期
	Success bool   `json:"success"`
	Msg     string `json:"msg,omitempty"`
}

// ExecuteByPeriod 按时间段执行分账：前置对账（C2 余额校验 + 合同校验）→ 解析规则 → 分配计算 → 落地账本。
func (s *Service) ExecuteByPeriod(ctx context.Context, req *ExecuteByPeriodRequest) (*ExecuteByPeriodResponse, error) {
	start, err := time.Parse(time.RFC3339, req.Start)
	if err != nil {
		return nil, errs.New(errs.CodeInvalidParams, "invalid start time", 200)
	}
	end, err := time.Parse(time.RFC3339, req.End)
	if err != nil {
		return nil, errs.New(errs.CodeInvalidParams, "invalid end time", 200)
	}
	if !end.After(start) {
		return nil, errs.New(errs.CodeInvalidParams, "end must be after start", 200)
	}
	now := time.Now()
	s.logger.Info("split period start",
		zap.Uint64("merchant_id", req.MerchantID),
		zap.String("start", req.Start),
		zap.String("end", req.End),
		zap.String("rule_code", req.RuleCode))

	// 1. 前置对账
	if s.prechecker != nil {
		if ok, msg := s.prechecker.Precheck(ctx, req.MerchantID, start, end); !ok {
			return nil, errs.New(errs.CodeSplitPrecheckFailed, msg, 200)
		}
	}

	// 2. 加载规则
	var rules []*rule.Rule
	if len(req.RuleIDs) > 0 {
		for _, id := range req.RuleIDs {
			r, rErr := s.ruleRepo.GetByID(ctx, id)
			if rErr != nil {
				return nil, errs.Wrap(errs.CodeInternalError, "query split rule failed", 200, rErr)
			}
			if r == nil || r.MerchantID != req.MerchantID {
				return nil, errs.New(errs.CodeSplitRuleNotMatch, fmt.Sprintf("rule %d not found", id), 200)
			}
			rules = append(rules, r)
		}
	} else if req.RuleCode != "" {
		r, rErr := s.ruleRepo.GetByCodeAndMerchant(ctx, req.RuleCode, req.MerchantID)
		if rErr != nil {
			return nil, errs.Wrap(errs.CodeInternalError, "query split rule failed", 200, rErr)
		}
		if r == nil {
			return nil, errs.New(errs.CodeSplitRuleNotMatch, "rule not found", 200)
		}
		rules = []*rule.Rule{r}
	} else {
		all, rErr := s.ruleRepo.ListByMerchant(ctx, req.MerchantID)
		if rErr != nil {
			return nil, errs.Wrap(errs.CodeInternalError, "load split rules failed", 200, rErr)
		}
		matchCtx := rule.MatchContext{
			MerchantID: req.MerchantID,
			Channel:    req.Channel,
			NowAt:      now.Format(time.RFC3339),
		}
		r := s.ruleEngine.Resolve(all, matchCtx)
		if r == nil {
			return nil, errs.New(errs.CodeSplitRuleNotMatch, "no matching rule", 200)
		}
		rules = []*rule.Rule{r}
	}

	// 3. 查询实收总额
	totalPaid, qErr := s.revenueQuerier.SumPaid(ctx, req.MerchantID, start, end)
	if qErr != nil {
		return nil, errs.Wrap(errs.CodeInternalError, "query paid total failed", 200, qErr)
	}
	if totalPaid <= 0 {
		return nil, errs.New(errs.CodeInvalidParams, "所选时间段内没有实收金额", 200)
	}

	// 4. 跨规则防护：禁止重复执行
	bizDates := s.collectBizDates(start, end)
	for _, bd := range bizDates {
		existing, eErr := s.orderStatusRepo.ExistsByBizDate(ctx, req.MerchantID, bd)
		if eErr != nil {
			return nil, errs.Wrap(errs.CodeInternalError, "check existing split failed", 200, eErr)
		}
		if existing {
			return nil, errs.New(errs.CodeSplitPeriodConflict, fmt.Sprintf("业务日期 %s 已执行过分账，请检查", bd), 200)
		}
	}

	// 5. 分配计算 + 落地
	revenues, _ := s.revenueQuerier.SumPaidByStore(ctx, req.MerchantID, start, end)
	merchantWallet, wErr := s.account.GetWalletByEntityType(ctx, req.MerchantID, vo.EntityMerchant)
	if wErr != nil {
		return nil, errs.Wrap(errs.CodeInternalError, "query merchant wallet failed", 200, wErr)
	}
	if merchantWallet == nil {
		return nil, errs.New(errs.CodeInternalError, "merchant wallet not found", 200)
	}

	execID := fmt.Sprintf("split:%s:%d", now.Format("20060102150405"), req.MerchantID)
	var failedRules []string
	for _, matched := range rules {
		allocations, aErr := alloc.Compute(alloc.Input{
			Rule:          matched,
			Total:         totalPaid,
			StoreRevenues: toAllocRevenues(revenues),
			FilterStoreIDs: req.StoreIDs,
		})
		if aErr != nil {
			s.logger.Error("分配计算失败", zap.Error(aErr), zap.String("rule", matched.RuleCode))
			failedRules = append(failedRules, matched.RuleCode)
			continue
		}
		execReq := &executor.ExecuteRequest{
			MerchantID:     req.MerchantID,
			OrderNo:        execID,
			SourceWallet:   merchantWallet.ID,
			Allocations:    allocations,
			StoreID:        0,
			RuleID:         matched.ID,
			Channel:        vo.ChannelCode(req.Channel),
			IdempotencyKey: "split",
			TraceID:        req.TraceID,
		}
		if eErr := s.executor.Execute(ctx, execReq); eErr != nil {
			s.logger.Error("分账执行失败", zap.Error(eErr), zap.String("rule", matched.RuleCode))
			failedRules = append(failedRules, matched.RuleCode)
		}
	}

	if len(failedRules) > 0 {
		errMsg := fmt.Sprintf("以下规则分账执行失败: %v", failedRules)
		return &ExecuteByPeriodResponse{BizDate: bizDates[0], Success: false, Msg: errMsg}, nil
	}
	return &ExecuteByPeriodResponse{BizDate: bizDates[0], Success: true}, nil
}

// GenerateBill 生成时段分账账单：基于已执行的分账记录生成审批账单（含快照）。
func (s *Service) GenerateBill(ctx context.Context, merchantID uint64, req *struct {
	Start string `json:"start" binding:"required"`
	End   string `json:"end" binding:"required"`
}) (*BillDTO, error) {
	start, err := time.Parse(time.RFC3339, req.Start)
	if err != nil {
		return nil, errs.New(errs.CodeInvalidParams, "invalid start time", 200)
	}
	end, err := time.Parse(time.RFC3339, req.End)
	if err != nil {
		return nil, errs.New(errs.CodeInvalidParams, "invalid end time", 200)
	}
	if !end.After(start) {
		return nil, errs.New(errs.CodeInvalidParams, "end must be after start", 200)
	}

	// 查规则
	all, rErr := s.ruleRepo.ListByMerchant(ctx, merchantID)
	if rErr != nil {
		return nil, errs.Wrap(errs.CodeInternalError, "load split rules failed", 200, rErr)
	}
	if len(all) == 0 {
		return nil, errs.New(errs.CodeSplitRuleNotMatch, "商户未配置分账规则", 200)
	}
	matched := s.ruleEngine.Resolve(all, rule.MatchContext{
		MerchantID: merchantID,
		NowAt:      time.Now().Format(time.RFC3339),
	})
	if matched == nil {
		return nil, errs.New(errs.CodeSplitRuleNotMatch, "未匹配到可用分账规则", 200)
	}

	// 查实收
	total, qErr := s.revenueQuerier.SumPaid(ctx, merchantID, start, end)
	if qErr != nil {
		return nil, errs.Wrap(errs.CodeInternalError, "query paid total failed", 200, qErr)
	}
	if total <= 0 {
		return nil, errs.New(errs.CodeInvalidParams, "所选时间段内没有实收金额", 200)
	}

	// 分配计算
	revenues, _ := s.revenueQuerier.SumPaidByStore(ctx, merchantID, start, end)
	allocations, aErr := alloc.Compute(alloc.Input{
		Rule:          matched,
		Total:         total,
		StoreRevenues: toAllocRevenues(revenues),
	})
	if aErr != nil {
		return nil, aErr
	}

	// 落快照
	snapshot, _ := json.Marshal(allocations)
	bill, err := s.billRepo.Create(ctx, &repository.SplitBill{
		MerchantID: merchantID,
		RuleID:     matched.ID,
		RuleCode:   matched.RuleCode,
		TotalAmount: total,
		Status:     repository.BillStatusPending,
		StartDate:  start,
		EndDate:    end,
		Snapshot:   string(snapshot),
	})
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternalError, "create split bill failed", 200, err)
	}
	return s.billToDTO(ctx, bill), nil
}

// BillDTO 分账账单 DTO（HTTP 响应）。
type BillDTO struct {
	ID          uint64         `json:"id"`
	BatchNo     string         `json:"batch_no"`
	MerchantID  uint64         `json:"merchant_id"`
	RuleCode    string         `json:"rule_code"`
	RuleName    string         `json:"rule_name"`
	StartTime   string         `json:"start_time"`
	EndTime     string         `json:"end_time"`
	TotalAmount int64          `json:"total_amount"`
	Status      string         `json:"status"`
	Items       []repository.SplitBillItem `json:"items,omitempty"`
	OrderNos    []string       `json:"order_nos,omitempty"`
	CreatedAt   string         `json:"created_at"`
	ApprovedAt  *string        `json:"approved_at,omitempty"`
	ExecutedAt  *string        `json:"executed_at,omitempty"`
}

// BillPage 账单分页结果。
type BillPage struct {
	Items []BillDTO `json:"items"`
	Total int64     `json:"total"`
}

// ListBills 分页查询商户分账账单。
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
		return nil, errs.Wrap(errs.CodeInternalError, "query split bills failed", 200, err)
	}
	out := make([]BillDTO, 0, len(items))
	for _, it := range items {
		out = append(out, *s.billToDTO(ctx, &it))
	}
	return &BillPage{Items: out, Total: total}, nil
}

// GetBillDetail 查询账单详情（含快照和明细）。
func (s *Service) GetBillDetail(ctx context.Context, merchantID uint64, batchNo string) (*BillDTO, error) {
	if s.billRepo == nil {
		return nil, errs.New(errs.CodeInternalError, "bill repo not configured", 500)
	}
	bill, err := s.billRepo.GetByBatchNo(ctx, batchNo, merchantID)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternalError, "query split bill failed", 200, err)
	}
	if bill == nil {
		return nil, errs.New(errs.CodeInvalidParams, "split bill not found", 200)
	}
	return billToDTO(bill), nil
}

// BillStoreItem 门店汇总条目。
type BillStoreItem struct {
	StoreID   uint64 `json:"store_id"`
	StoreName string `json:"store_name"`
	PaidTotal int64  `json:"paid_total"`  // 门店实收总额（分）
	SplitTotal int64 `json:"split_total"` // 门店分账总额（分）
}

// BillStoreSummary 门店汇总响应。
type BillStoreSummary struct {
	StoreItems []BillStoreItem `json:"store_items"`
	TotalPaid  int64           `json:"total_paid"`
	TotalSplit int64           `json:"total_split"`
}

// BillStoreSummary 查询账单门店级别汇总：每门店实收 vs 分账。
func (s *Service) BillStoreSummary(ctx context.Context, merchantID uint64, batchNo string) (*BillStoreSummary, error) {
	if s.billRepo == nil {
		return nil, errs.New(errs.CodeInternalError, "bill repo not configured", 500)
	}
	bill, err := s.billRepo.GetByBatchNo(ctx, batchNo, merchantID)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternalError, "query split bill failed", 200, err)
	}
	if bill == nil {
		return nil, errs.New(errs.CodeInvalidParams, "split bill not found", 200)
	}

	// 从快照反解析分配
	var allocations []executor.Allocation
	if err := json.Unmarshal([]byte(bill.Snapshot), &allocations); err != nil {
		return nil, errs.Wrap(errs.CodeInternalError, "parse bill snapshot failed", 200, err)
	}

	// 按门店汇总
	storeMap := make(map[uint64]int64)
	for _, a := range allocations {
		if a.EntityType == vo.EntityStore {
			storeMap[a.EntityID] += a.Amount
		}
	}

	// 查门店名称
	storeIDs := make([]uint64, 0, len(storeMap))
	for id := range storeMap {
		storeIDs = append(storeIDs, id)
	}
	storeNames, _ := s.billRepo.GetStoreNames(ctx, storeIDs)
	nameMap := make(map[uint64]string)
	for _, s := range storeNames {
		nameMap[s.ID] = s.Name
	}

	items := make([]BillStoreItem, 0, len(storeMap))
	var totalPaid, totalSplit int64
	for id, splitAmt := range storeMap {
		items = append(items, BillStoreItem{
			StoreID:    id,
			StoreName:  nameMap[id],
			SplitTotal: splitAmt,
		})
		totalSplit += splitAmt
	}
	totalPaid = bill.TotalAmount

	return &BillStoreSummary{StoreItems: items, TotalPaid: totalPaid, TotalSplit: totalSplit}, nil
}

// BillStoreOrder 门店订单行。
type BillStoreOrder struct {
	OrderNo  string `json:"order_no"`
	Paid     int64  `json:"paid"`
	Split    int64  `json:"split"`
	Status   string `json:"status"`
	PaidAt   string `json:"paid_at"`
}

// BillStoreOrders 门店订单级明细响应。
type BillStoreOrders struct {
	StoreID   uint64          `json:"store_id"`
	StoreName string          `json:"store_name"`
	Orders    []BillStoreOrder `json:"orders"`
	TotalPaid int64           `json:"total_paid"`
	TotalSplit int64          `json:"total_split"`
}

// BillStoreOrders 查询账单中某门店的订单级明细。
func (s *Service) BillStoreOrders(ctx context.Context, merchantID uint64, batchNo string, storeID uint64) (*BillStoreOrders, error) {
	if s.billRepo == nil || s.orderStatusRepo == nil {
		return nil, errs.New(errs.CodeInternalError, "bill repo not configured", 500)
	}
	bill, err := s.billRepo.GetByBatchNo(ctx, batchNo, merchantID)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternalError, "query split bill failed", 200, err)
	}
	if bill == nil {
		return nil, errs.New(errs.CodeInvalidParams, "split bill not found", 200)
	}

	// 查该门店在该时间段内的订单
	orders, err := s.billRepo.GetStoreOrders(ctx, storeID, bill.StartTime, bill.EndTime)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternalError, "query store orders failed", 200, err)
	}

	items := make([]BillStoreOrder, 0, len(orders))
	var totalPaid, totalSplit int64
	for _, o := range orders {
		items = append(items, BillStoreOrder{
			OrderNo: o.OrderNo,
			Paid:    o.Amount,
			Status:  o.Status,
			PaidAt:  o.PaidAt.Format(time.RFC3339),
		})
		totalPaid += o.Amount
	}

	// 查分账记录
	execRows, err := s.orderStatusRepo.ListByStoreID(ctx, storeID, bill.StartTime, bill.EndTime)
	if err == nil {
		for _, r := range execRows {
			totalSplit += r.Amount
		}
	}

	storeNames, _ := s.billRepo.GetStoreNames(ctx, []uint64{storeID})
	storeName := ""
	if len(storeNames) > 0 {
		storeName = storeNames[0].Name
	}

	return &BillStoreOrders{
		StoreID:    storeID,
		StoreName:  storeName,
		Orders:     items,
		TotalPaid:  totalPaid,
		TotalSplit: totalSplit,
	}, nil
}

// ApproveBill 审批通过账单。
func (s *Service) ApproveBill(ctx context.Context, merchantID uint64, batchNo string) error {
	// 1. 查账单
	bill, err := s.getPendingBillRaw(ctx, merchantID, batchNo)
	if err != nil {
		return err
	}

	// 2. 解析快照
	var allocations []executor.Allocation
	if err := json.Unmarshal([]byte(bill.Snapshot), &allocations); err != nil {
		return errs.Wrap(errs.CodeInternalError, "parse bill snapshot failed", 200, err)
	}

	// 3. 查钱包
	merchantWallet, wErr := s.account.GetWalletByEntityType(ctx, merchantID, vo.EntityMerchant)
	if wErr != nil || merchantWallet == nil {
		return errs.New(errs.CodeInternalError, "merchant wallet not found", 200)
	}

	// 4. 执行分账
	execID := fmt.Sprintf("approve:%s:%s", batchNo, time.Now().Format("20060102150405"))
	if eErr := s.executor.Execute(ctx, &executor.ExecuteRequest{
		MerchantID:     merchantID,
		OrderNo:        execID,
		SourceWallet:   merchantWallet.ID,
		Allocations:    allocations,
		RuleID:         bill.RuleID,
		IdempotencyKey: "split",
	}); eErr != nil {
		return errs.Wrap(errs.CodeInternalError, "approve split bill execute failed", 200, eErr)
	}

	// 5. 更新账单状态
	updErr := s.billRepo.UpdateStatus(ctx, bill.ID, repository.BillStatusApproved)
	if updErr != nil {
		s.logger.Error("update bill status failed", zap.Error(updErr), zap.String("batch_no", batchNo))
	}
	s.appendAudit(ctx, "BILL", batchNo, "APPROVE", merchantID, nil)
	return nil
}

// RejectBill 驳回账单。
func (s *Service) RejectBill(ctx context.Context, merchantID uint64, batchNo string) error {
	_, _, err := s.getPendingBillRaw(ctx, merchantID, batchNo)
	if err != nil {
		return err
	}
	if err := s.billRepo.UpdateStatus(ctx, batchNo, repository.BillStatusRejected); err != nil {
		return errs.Wrap(errs.CodeInternalError, "update bill status failed", 200, err)
	}
	s.appendAudit(ctx, "BILL", batchNo, "REJECT", merchantID, nil)
	return nil
}