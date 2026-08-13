// 包 service 编排分账业务。
package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/huipay/huipay-backend/infra/errs"
	"github.com/huipay/huipay-backend/internal/account/service"
	"github.com/huipay/huipay-backend/internal/domain/vo"
	"github.com/huipay/huipay-backend/internal/split/executor"
	"github.com/huipay/huipay-backend/internal/split/repository"
	"github.com/huipay/huipay-backend/internal/split/rule"
	"go.uber.org/zap"
)

// ExecuteRequest 分账执行请求（HTTP 层）。
type ExecuteRequest struct {
	OrderNo    string             `json:"order_no" binding:"required"`
	MerchantID uint64             `json:"merchant_id"` // 由 handler 从登录上下文填充，忽略请求体
	Amount     int64              `json:"amount" binding:"required,gt=0"`
	RuleCode   string             `json:"rule_code"` // 可选：指定规则
	StoreID    uint64             `json:"store_id"`
	Channel    vo.ChannelCode     `json:"channel"`
	PaidAt     string             `json:"paid_at"` // RFC3339，订单支付时间（全门店分摊按此时间截取实收）
	TraceID    string             `json:"-"`
}

// ExecuteResponse 分账执行响应。
type ExecuteResponse struct {
	OrderNo    string                       `json:"order_no"`
	Allocations []executor.Allocation       `json:"allocations"`
	RuleCode   string                       `json:"rule_code"`
}

// Service 分账服务。
type Service struct {
	ruleEngine     *rule.Engine
	executor       *executor.Executor
	ruleRepo       *repository.SplitRuleRepo
	billRepo       *repository.SplitBillRepo
	account        *service.Service
	revenueQuerier repository.StoreRevenueQuerier
	logger         *zap.Logger
}

// NewService 构造 Service。
func NewService(re *rule.Engine, ex *executor.Executor, ruleRepo *repository.SplitRuleRepo, billRepo *repository.SplitBillRepo, acc *service.Service, revQuerier repository.StoreRevenueQuerier, logger *zap.Logger) *Service {
	return &Service{ruleEngine: re, executor: ex, ruleRepo: ruleRepo, billRepo: billRepo, account: acc, revenueQuerier: revQuerier, logger: logger}
}

// Execute 执行分账：按规则引擎匹配（支持门店维度），解析分配方案后落地账本。
func (s *Service) Execute(ctx context.Context, req *ExecuteRequest) (*ExecuteResponse, error) {
	if s.ruleRepo == nil {
		return nil, errs.New(errs.CodeInternalError, "split rule repo not configured", 500)
	}
	// 1) 确认具备规则引擎与执行器
	if s.ruleEngine == nil || s.executor == nil {
		return nil, errs.New(errs.CodeInternalError, "split engine not configured", 500)
	}

	// 2) 匹配规则（指定 RuleCode 直选，否则按宿主商户 + 门店 + 通道匹配）
	var matched *rule.Rule
	var err error
	if req.RuleCode != "" {
		matched, err = s.ruleRepo.GetByCodeAndMerchant(ctx, req.RuleCode, req.MerchantID)
		if err != nil {
			return nil, errs.Wrap(errs.CodeInternalError, "query split rule failed", 200, err)
		}
		if matched == nil {
			return nil, errs.New(errs.CodeSplitRuleNotMatch, "split rule not found", 200)
		}
	} else {
		rules, rErr := s.ruleRepo.ListByMerchant(ctx, req.MerchantID)
		if rErr != nil {
			return nil, errs.Wrap(errs.CodeInternalError, "load split rules failed", 200, rErr)
		}
		matched = s.ruleEngine.Resolve(rules, rule.MatchContext{
			MerchantID: req.MerchantID,
			Channel:    string(req.Channel),
			StoreID:    req.StoreID,
			NowAt:      time.Now().Format(time.RFC3339),
		})
		if matched == nil {
			return nil, errs.New(errs.CodeSplitRuleNotMatch, "no matching split rule", 200)
		}
	}

	// 3) 由规则分配方案计算金额并映射为执行单元
	allocations, aErr := s.buildAllocations(ctx, matched, req.Amount, parsePaidAt(req.PaidAt))
	if aErr != nil {
		return nil, aErr
	}

	// 4) 源账户：商户钱包（订单款项已结算入商户钱包，分账从商户钱包扣减）
	if s.account == nil {
		return nil, errs.New(errs.CodeInternalError, "account service not configured", 500)
	}
	merchantWallet, wErr := s.account.GetWalletByEntityType(ctx, req.MerchantID, vo.EntityMerchant)
	if wErr != nil || merchantWallet == nil {
		return nil, errs.New(errs.CodeInternalError, "merchant wallet not found", 200)
	}

	// 5) 执行落地
	if err := s.executor.Execute(ctx, &executor.ExecuteRequest{
		OrderNo:       req.OrderNo,
		SourceWallet:  merchantWallet.ID,
		Allocations:   allocations,
		StoreID:       req.StoreID,
		RuleID:        matched.ID,
		Channel:       req.Channel,
		IdempotencyKey: "split",
		TraceID:       req.TraceID,
	}); err != nil {
		return nil, errs.Wrap(errs.CodeInternalError, "split execute failed", 200, err)
	}
	return &ExecuteResponse{OrderNo: req.OrderNo, Allocations: allocations, RuleCode: matched.RuleCode}, nil
}

// ExecuteByPeriodRequest 按时间段分账请求。
type ExecuteByPeriodRequest struct {
	Start    string `json:"start" binding:"required"` // RFC3339，起始时间
	End      string `json:"end" binding:"required"`   // RFC3339，结束时间
	RuleCode string `json:"rule_code" binding:"required"`
}

// ExecuteByPeriodResponse 按时间段分账响应。
type ExecuteByPeriodResponse struct {
	BatchNo     string                 `json:"batch_no"`
	TotalAmount int64                  `json:"total_amount"` // 时间段内商户实收总额（分）
	RuleCode    string                 `json:"rule_code"`
	Allocations []executor.Allocation  `json:"allocations"`
}

// ExecuteByPeriod 按时间段分账：选定规则，以时间段内商户实收总额为基数，按门店时间段实收占比分配。
// 批次号由 规则+起止时间 确定性生成，同一时间段同一规则重复执行会被 executor 幂等跳过，避免重复分账。
func (s *Service) ExecuteByPeriod(ctx context.Context, merchantID uint64, req *ExecuteByPeriodRequest) (*ExecuteByPeriodResponse, error) {
	start, err1 := time.Parse(time.RFC3339, req.Start)
	end, err2 := time.Parse(time.RFC3339, req.End)
	if err1 != nil || err2 != nil {
		return nil, errs.New(errs.CodeInvalidParams, "invalid start/end time", 200)
	}
	if !end.After(start) {
		return nil, errs.New(errs.CodeInvalidParams, "end must be after start", 200)
	}
	if s.ruleRepo == nil || s.executor == nil {
		return nil, errs.New(errs.CodeInternalError, "split engine not configured", 500)
	}

	// 选定规则
	matched, err := s.ruleRepo.GetByCodeAndMerchant(ctx, req.RuleCode, merchantID)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternalError, "query split rule failed", 200, err)
	}
	if matched == nil {
		return nil, errs.New(errs.CodeSplitRuleNotMatch, "split rule not found", 200)
	}

	// 时间段内商户实收总额（分账基数）
	if s.revenueQuerier == nil {
		return nil, errs.New(errs.CodeInternalError, "store revenue querier not configured", 500)
	}
	total, err := s.revenueQuerier.SumPaid(ctx, merchantID, start, end)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternalError, "query paid total failed", 200, err)
	}
	if total <= 0 {
		return nil, errs.New(errs.CodeInvalidParams, "no paid amount in period", 200)
	}

	// 按规则分配方案换算（门店占比基于时间段实收）
	allocations, aErr := s.buildAllocationsPeriod(ctx, matched, total, start, end)
	if aErr != nil {
		return nil, aErr
	}

	// 源账户：商户钱包，校验余额足够覆盖时间段实收总额
	if s.account == nil {
		return nil, errs.New(errs.CodeInternalError, "account service not configured", 500)
	}
	merchantWallet, wErr := s.account.GetWalletByEntityType(ctx, merchantID, vo.EntityMerchant)
	if wErr != nil || merchantWallet == nil {
		return nil, errs.New(errs.CodeInternalError, "merchant wallet not found", 200)
	}
	if merchantWallet.Balance < total {
		return nil, errs.New(errs.CodeInsufficientBalance, "insufficient wallet balance for period split", 200)
	}

	// 批次号：确定性（同一时间段+规则幂等），供分账记录聚合展示
	batchNo := fmt.Sprintf("SP%d-%d-%d", matched.ID, start.Unix(), end.Unix())

	if err := s.executor.Execute(ctx, &executor.ExecuteRequest{
		OrderNo:        batchNo,
		SourceWallet:   merchantWallet.ID,
		Allocations:    allocations,
		RuleID:         matched.ID,
		IdempotencyKey: "split",
		TraceID:        "",
	}); err != nil {
		return nil, errs.Wrap(errs.CodeInternalError, "split execute failed", 200, err)
	}
	return &ExecuteByPeriodResponse{
		BatchNo:     batchNo,
		TotalAmount: total,
		RuleCode:    matched.RuleCode,
		Allocations: allocations,
	}, nil
}

// buildAllocationsPeriod 将规则分配方案换算为金额执行单元（门店占比基于时间段 [start, end] 实收）。
func (s *Service) buildAllocationsPeriod(ctx context.Context, r *rule.Rule, total int64, start, end time.Time) ([]executor.Allocation, error) {
	if len(r.Allocations) == 0 {
		return nil, errs.New(errs.CodeSplitRuleNotMatch, "split rule has no allocations", 200)
	}
	allocations := make([]executor.Allocation, 0, len(r.Allocations))
	var ratioSum int64
	for _, a := range r.Allocations {
		ratioSum += a.RatioBps
	}
	var used int64
	for i, a := range r.Allocations {
		amount := a.FixedAmount
		if a.RatioBps > 0 {
			amount = total * a.RatioBps / 10000
		}
		if amount <= 0 {
			return nil, errs.New(errs.CodeInvalidParams, "invalid allocation amount", 200)
		}
		if i == len(r.Allocations)-1 && ratioSum == 10000 {
			remain := total - used
			if remain > 0 && remain != amount {
				amount = remain
			}
		}
		used += amount
		if a.ReceiverScope == "ALL_STORES" {
			expanded, err := s.expandAllStores(ctx, r.MerchantID, amount, i+1, start, end)
			if err != nil {
				return nil, err
			}
			allocations = append(allocations, expanded...)
			continue
		}
		allocations = append(allocations, executor.Allocation{
			Level:      i + 1,
			EntityID:   a.ReceiverEntityID,
			EntityType: vo.EntityType(a.ReceiverType),
			Amount:     amount,
		})
	}
	if used > total {
		return nil, errs.New(errs.CodeInvalidParams, "allocations exceed period amount", 200)
	}
	return allocations, nil
}

// BillDTO 分账单视图（HTTP 返回）。
type BillDTO struct {
	ID          uint64                       `json:"id"`
	BatchNo     string                       `json:"batch_no"`
	RuleCode    string                       `json:"rule_code"`
	RuleName    string                       `json:"rule_name"`
	StartTime   string                       `json:"start_time"`
	EndTime     string                       `json:"end_time"`
	TotalAmount int64                        `json:"total_amount"`
	Status      string                       `json:"status"`
	Items       []repository.SplitBillItem   `json:"items"`
	CreatedAt   string                       `json:"created_at"`
	ApprovedAt  *string                      `json:"approved_at"`
	ExecutedAt  *string                      `json:"executed_at"`
}

// BillPage 分账单分页结果。
type BillPage struct {
	Items []BillDTO `json:"items"`
	Total int64     `json:"total"`
}

// GenerateBill 生成分账单：选定规则，计算时间段内各门店可分金额，保存为待审批账单（不扣款）。
func (s *Service) GenerateBill(ctx context.Context, merchantID uint64, req *ExecuteByPeriodRequest) (*BillDTO, error) {
	start, err1 := time.Parse(time.RFC3339, req.Start)
	end, err2 := time.Parse(time.RFC3339, req.End)
	if err1 != nil || err2 != nil {
		return nil, errs.New(errs.CodeInvalidParams, "invalid start/end time", 200)
	}
	if !end.After(start) {
		return nil, errs.New(errs.CodeInvalidParams, "end must be after start", 200)
	}
	if s.ruleRepo == nil || s.billRepo == nil {
		return nil, errs.New(errs.CodeInternalError, "split repo not configured", 500)
	}

	matched, err := s.ruleRepo.GetByCodeAndMerchant(ctx, req.RuleCode, merchantID)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternalError, "query split rule failed", 200, err)
	}
	if matched == nil {
		return nil, errs.New(errs.CodeSplitRuleNotMatch, "split rule not found", 200)
	}

	if s.revenueQuerier == nil {
		return nil, errs.New(errs.CodeInternalError, "store revenue querier not configured", 500)
	}
	total, err := s.revenueQuerier.SumPaid(ctx, merchantID, start, end)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternalError, "query paid total failed", 200, err)
	}
	if total <= 0 {
		return nil, errs.New(errs.CodeInvalidParams, "no paid amount in period", 200)
	}

	allocations, aErr := s.buildAllocationsPeriod(ctx, matched, total, start, end)
	if aErr != nil {
		return nil, aErr
	}

	items := allocationsToItems(allocations)
	if err := s.fillBillItemNames(ctx, items); err != nil {
		return nil, errs.Wrap(errs.CodeInternalError, "fill receiver names failed", 200, err)
	}

	detailJSON, err := json.Marshal(items)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInvalidParams, "marshal bill detail failed", 200, err)
	}

	batchNo := fmt.Sprintf("SP%d-%d-%d", matched.ID, start.Unix(), end.Unix())
	m := &repository.SplitBillModel{
		BatchNo:     batchNo,
		MerchantID:  merchantID,
		RuleCode:    matched.RuleCode,
		RuleName:    matched.RuleName,
		StartTime:   start,
		EndTime:     end,
		TotalAmount: total,
		Detail:      string(detailJSON),
		Status:      repository.BillPending,
	}
	if err := s.billRepo.Create(ctx, m); err != nil {
		return nil, errs.Wrap(errs.CodeInternalError, "create split bill failed", 200, err)
	}
	return billToDTO(m), nil
}

// ListBills 分页查询商户分账单。
func (s *Service) ListBills(ctx context.Context, merchantID uint64, page, size int) (*BillPage, error) {
	if s.billRepo == nil {
		return nil, errs.New(errs.CodeInternalError, "split bill repo not configured", 500)
	}
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 200 {
		size = 20
	}
	rows, total, err := s.billRepo.ListByMerchant(ctx, merchantID, (page-1)*size, size)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternalError, "query split bills failed", 200, err)
	}
	out := make([]BillDTO, 0, len(rows))
	for i := range rows {
		out = append(out, *billToDTO(&rows[i]))
	}
	return &BillPage{Items: out, Total: total}, nil
}

// GetBillDetail 查询分账单详情（含各门店可分金额明细）。
func (s *Service) GetBillDetail(ctx context.Context, merchantID uint64, batchNo string) (*BillDTO, error) {
	if s.billRepo == nil {
		return nil, errs.New(errs.CodeInternalError, "split bill repo not configured", 500)
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

// ApproveBill 审批通过分账单并执行：校验余额后调用 executor 转账，账单状态置 EXECUTED。
func (s *Service) ApproveBill(ctx context.Context, merchantID uint64, batchNo string) (*BillDTO, error) {
	bill, err := s.getPendingBill(ctx, merchantID, batchNo)
	if err != nil {
		return nil, err
	}
	var items []repository.SplitBillItem
	if err := json.Unmarshal([]byte(bill.Detail), &items); err != nil {
		return nil, errs.New(errs.CodeInternalError, "parse bill detail failed", 200)
	}
	allocations := itemsToAllocations(items)

	if s.account == nil || s.executor == nil {
		return nil, errs.New(errs.CodeInternalError, "split executor not configured", 500)
	}
	merchantWallet, wErr := s.account.GetWalletByEntityType(ctx, merchantID, vo.EntityMerchant)
	if wErr != nil || merchantWallet == nil {
		return nil, errs.New(errs.CodeInternalError, "merchant wallet not found", 200)
	}
	if merchantWallet.Balance < bill.TotalAmount {
		return nil, errs.New(errs.CodeInsufficientBalance, "insufficient wallet balance for split", 200)
	}

	if err := s.executor.Execute(ctx, &executor.ExecuteRequest{
		OrderNo:        bill.BatchNo,
		SourceWallet:   merchantWallet.ID,
		Allocations:    allocations,
		RuleID:         0,
		IdempotencyKey: "split",
		TraceID:        "",
	}); err != nil {
		return nil, errs.Wrap(errs.CodeInternalError, "split execute failed", 200, err)
	}

	now := time.Now()
	if err := s.billRepo.UpdateStatus(ctx, bill.ID, repository.BillExecuted, map[string]any{
		"approved_at": now,
		"executed_at": now,
	}); err != nil {
		return nil, errs.Wrap(errs.CodeInternalError, "update split bill failed", 200, err)
	}
	bill.Status = repository.BillExecuted
	bill.ApprovedAt = &now
	bill.ExecutedAt = &now
	return billToDTO(bill), nil
}

// RejectBill 驳回分账单（仅待审批状态可驳回）。
func (s *Service) RejectBill(ctx context.Context, merchantID uint64, batchNo string) (*BillDTO, error) {
	bill, _, err := s.getPendingBillRaw(ctx, merchantID, batchNo)
	if err != nil {
		return nil, err
	}
	if err := s.billRepo.UpdateStatus(ctx, bill.ID, repository.BillRejected, nil); err != nil {
		return nil, errs.Wrap(errs.CodeInternalError, "update split bill failed", 200, err)
	}
	bill.Status = repository.BillRejected
	return billToDTO(bill), nil
}

// getPendingBill 查询待审批账单（校验状态）。
func (s *Service) getPendingBill(ctx context.Context, merchantID uint64, batchNo string) (*repository.SplitBillModel, error) {
	bill, _, err := s.getPendingBillRaw(ctx, merchantID, batchNo)
	if err != nil {
		return nil, err
	}
	return bill, nil
}

func (s *Service) getPendingBillRaw(ctx context.Context, merchantID uint64, batchNo string) (*repository.SplitBillModel, bool, error) {
	if s.billRepo == nil {
		return nil, false, errs.New(errs.CodeInternalError, "split bill repo not configured", 500)
	}
	bill, err := s.billRepo.GetByBatchNo(ctx, batchNo, merchantID)
	if err != nil {
		return nil, false, errs.Wrap(errs.CodeInternalError, "query split bill failed", 200, err)
	}
	if bill == nil {
		return nil, false, errs.New(errs.CodeInvalidParams, "split bill not found", 200)
	}
	if bill.Status != repository.BillPending {
		return nil, false, errs.New(errs.CodeInvalidParams, "bill is not pending", 200)
	}
	return bill, true, nil
}

// fillBillItemNames 回填明细接收方名称（STORE->t_store、MERCHANT->t_entity、其他#id）。
func (s *Service) fillBillItemNames(ctx context.Context, items []repository.SplitBillItem) error {
	var storeIDs, entIDs []uint64
	for _, it := range items {
		if it.ReceiverType == string(vo.EntityStore) {
			storeIDs = append(storeIDs, it.ReceiverEntityID)
		} else if it.ReceiverType == string(vo.EntityMerchant) {
			entIDs = append(entIDs, it.ReceiverEntityID)
		}
	}
	storeNames, err := s.billRepo.GetStoreNames(ctx, storeIDs)
	if err != nil {
		return err
	}
	entNames, err := s.billRepo.GetEntityNames(ctx, entIDs)
	if err != nil {
		return err
	}
	for i := range items {
		switch items[i].ReceiverType {
		case string(vo.EntityStore):
			items[i].ReceiverName = storeNames[items[i].ReceiverEntityID]
		case string(vo.EntityMerchant):
			items[i].ReceiverName = entNames[items[i].ReceiverEntityID]
		default:
			items[i].ReceiverName = fmt.Sprintf("#%d", items[i].ReceiverEntityID)
		}
	}
	return nil
}

// allocationsToItems 将执行单元转换为账单明细。
func allocationsToItems(list []executor.Allocation) []repository.SplitBillItem {
	items := make([]repository.SplitBillItem, 0, len(list))
	for _, a := range list {
		items = append(items, repository.SplitBillItem{
			ReceiverEntityID: a.EntityID,
			ReceiverType:     string(a.EntityType),
			Amount:           a.Amount,
		})
	}
	return items
}

// itemsToAllocations 将账单明细转换为执行单元。
func itemsToAllocations(items []repository.SplitBillItem) []executor.Allocation {
	list := make([]executor.Allocation, 0, len(items))
	for _, it := range items {
		list = append(list, executor.Allocation{
			Level:      1,
			EntityID:   it.ReceiverEntityID,
			EntityType: vo.EntityType(it.ReceiverType),
			Amount:     it.Amount,
		})
	}
	return list
}

// billToDTO 模型转视图。
func billToDTO(m *repository.SplitBillModel) *BillDTO {
	var items []repository.SplitBillItem
	_ = json.Unmarshal([]byte(m.Detail), &items)
	dto := &BillDTO{
		ID:          m.ID,
		BatchNo:     m.BatchNo,
		RuleCode:    m.RuleCode,
		RuleName:    m.RuleName,
		StartTime:   m.StartTime.Format(time.RFC3339),
		EndTime:     m.EndTime.Format(time.RFC3339),
		TotalAmount: m.TotalAmount,
		Status:      m.Status,
		Items:       items,
		CreatedAt:   m.CreatedAt.Format(time.RFC3339),
	}
	if m.ApprovedAt != nil {
		s := m.ApprovedAt.Format(time.RFC3339)
		dto.ApprovedAt = &s
	}
	if m.ExecutedAt != nil {
		s := m.ExecutedAt.Format(time.RFC3339)
		dto.ExecutedAt = &s
	}
	return dto
}

// buildAllocations 将规则分配方案（比例/固定额）换算为金额执行单元。
// 分配项 receiver_scope=ALL_STORES 时按全门店实收占比展开为逐店子分配。
func (s *Service) buildAllocations(ctx context.Context, r *rule.Rule, total int64, paidAt time.Time) ([]executor.Allocation, error) {
	if len(r.Allocations) == 0 {
		return nil, errs.New(errs.CodeSplitRuleNotMatch, "split rule has no allocations", 200)
	}
	allocations := make([]executor.Allocation, 0, len(r.Allocations))
	// 比例总和（万分比），仅当分配项按比例恰好合计 100% 时才对末笔补齐取整误差
	var ratioSum int64
	for _, a := range r.Allocations {
		ratioSum += a.RatioBps
	}
	var used int64
	for i, a := range r.Allocations {
		amount := a.FixedAmount
		if a.RatioBps > 0 {
			amount = total * a.RatioBps / 10000
		}
		if amount <= 0 {
			return nil, errs.New(errs.CodeInvalidParams, "invalid allocation amount", 200)
		}
		// 末笔补齐仅用于比例合计 100% 时的取整误差；部分分账（合计 < 100%）不补齐
		if i == len(r.Allocations)-1 && ratioSum == 10000 {
			remain := total - used
			if remain > 0 && remain != amount {
				amount = remain
			}
		}
		used += amount

		if a.ReceiverScope == "ALL_STORES" {
			expanded, err := s.expandAllStores(ctx, r.MerchantID, amount, i+1, time.Time{}, paidAt)
			if err != nil {
				return nil, err
			}
			allocations = append(allocations, expanded...)
			continue
		}

		allocations = append(allocations, executor.Allocation{
			Level:      i + 1,
			EntityID:   a.ReceiverEntityID,
			EntityType: vo.EntityType(a.ReceiverType),
			Amount:     amount,
		})
	}
	if used > total {
		return nil, errs.New(errs.CodeInvalidParams, "allocations exceed order amount", 200)
	}
	return allocations, nil
}

// parsePaidAt 解析订单支付时间（RFC3339），解析失败返回零值（不按时间截取，查全部实收）。
func parsePaidAt(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

// expandAllStores 将某分配项金额按门店实收占比拆分为逐店子分配（时间范围 [from, to]）。
func (s *Service) expandAllStores(ctx context.Context, merchantID uint64, amount int64, level int, from, to time.Time) ([]executor.Allocation, error) {
	if s.revenueQuerier == nil {
		return nil, errs.New(errs.CodeInternalError, "store revenue querier not configured", 500)
	}
	revenues, err := s.revenueQuerier.SumPaidByStore(ctx, merchantID, from, to)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternalError, "query store revenue failed", 200, err)
	}
	// 过滤实收为 0 的门店
	valid := make([]repository.StoreRevenue, 0, len(revenues))
	var sum int64
	for _, rv := range revenues {
		if rv.Paid > 0 {
			valid = append(valid, rv)
			sum += rv.Paid
		}
	}
	if len(valid) == 0 || sum <= 0 {
		return nil, errs.New(errs.CodeInvalidParams, "no store revenue for split", 200)
	}
	// 按 store_id 排序，保证展开顺序确定
	sort.Slice(valid, func(i, j int) bool { return valid[i].StoreID < valid[j].StoreID })

	out := make([]executor.Allocation, 0, len(valid))
	var allocated int64
	for i, rv := range valid {
		sub := amount * rv.Paid / sum
		// 最末门店补足剩余，避免取整丢分
		if i == len(valid)-1 {
			sub = amount - allocated
		}
		if sub <= 0 {
			continue
		}
		allocated += sub
		out = append(out, executor.Allocation{
			Level:      level,
			EntityID:   rv.StoreID,
			EntityType: vo.EntityStore,
			Amount:     sub,
		})
	}
	return out, nil
}

// Get 查询分账结果（从分账执行记录读取）。
func (s *Service) Get(ctx context.Context, orderNo string) (*ExecuteResponse, error) {
	if orderNo == "" {
		return nil, errors.New("order_no required")
	}
	if s.executor == nil {
		return nil, errs.New(errs.CodeInternalError, "split executor not configured", 500)
	}
	rows, err := s.executor.ListByOrderNo(ctx, orderNo)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternalError, "query split execution failed", 200, err)
	}
	resp := &ExecuteResponse{OrderNo: orderNo, Allocations: make([]executor.Allocation, 0, len(rows))}
	for _, r := range rows {
		resp.Allocations = append(resp.Allocations, executor.Allocation{
			Level:      r.Level,
			EntityID:   r.ReceiverEntityID,
			EntityType: vo.EntityType(r.ReceiverType),
			Amount:     r.Amount,
		})
	}
	return resp, nil
}

// ExecutionPage 分账记录分页结果。
type ExecutionPage struct {
	Items []executor.SplitExecutionSummary `json:"items"`
	Total int64                            `json:"total"`
}

// ListExecutions 分页查询商户分账记录（按订单聚合）。
func (s *Service) ListExecutions(ctx context.Context, merchantID uint64, page, size int) (*ExecutionPage, error) {
	if s.executor == nil {
		return nil, errs.New(errs.CodeInternalError, "split executor not configured", 500)
	}
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 200 {
		size = 20
	}
	items, total, err := s.executor.ListByMerchant(ctx, merchantID, (page-1)*size, size)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternalError, "query split executions failed", 200, err)
	}
	return &ExecutionPage{Items: items, Total: total}, nil
}

// GetExecutionDetail 查询某订单分账明细（含商户隔离校验）。
func (s *Service) GetExecutionDetail(ctx context.Context, merchantID uint64, orderNo string) ([]executor.SplitExecutionDetail, error) {
	if s.executor == nil {
		return nil, errs.New(errs.CodeInternalError, "split executor not configured", 500)
	}
	rows, err := s.executor.ListByOrderNoWithReceiver(ctx, merchantID, orderNo)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternalError, "query split execution detail failed", 200, err)
	}
	if rows == nil {
		return nil, errs.New(errs.CodeInvalidParams, "split execution not found", 200)
	}
	return rows, nil
}

// RuleDTO 分账规则视图（HTTP 返回）。
type RuleDTO struct {
	ID          uint64            `json:"id"`
	RuleCode    string            `json:"rule_code"`
	RuleName    string            `json:"rule_name"`
	MerchantID  uint64            `json:"merchant_id"`
	Priority    int               `json:"priority"`
	Conditions  rule.Condition    `json:"conditions"`
	Allocations []rule.Allocation `json:"allocations"`
	Status      int               `json:"status"`
}

// CreateRuleRequest 创建分账规则请求。
type CreateRuleRequest struct {
	RuleCode    string            `json:"rule_code" binding:"required"`
	RuleName    string            `json:"rule_name" binding:"required"`
	Priority    int               `json:"priority"`
	Conditions  rule.Condition    `json:"conditions"`
	Allocations []rule.Allocation `json:"allocations" binding:"required"`
	Status      int               `json:"status"`
}

// UpdateRuleRequest 更新分账规则请求。
type UpdateRuleRequest struct {
	RuleName    *string           `json:"rule_name"`
	Priority    *int              `json:"priority"`
	Conditions  *rule.Condition   `json:"conditions"`
	Allocations *[]rule.Allocation `json:"allocations"`
}

// ListRules 查询商户分账规则（含启用与停用）。
func (s *Service) ListRules(ctx context.Context, merchantID uint64) ([]RuleDTO, error) {
	rules, err := s.ruleRepo.ListByMerchant(ctx, merchantID)
	if err != nil {
		return nil, err
	}
	out := make([]RuleDTO, 0, len(rules))
	for _, r := range rules {
		out = append(out, RuleDTO{
			ID:          r.ID,
			RuleCode:    r.RuleCode,
			RuleName:    r.RuleName,
			MerchantID:  r.MerchantID,
			Priority:    r.Priority,
			Conditions:  r.Conditions,
			Allocations: r.Allocations,
			Status:      r.Status,
		})
	}
	return out, nil
}

// CreateRule 创建分账规则。
func (s *Service) CreateRule(ctx context.Context, merchantID uint64, req *CreateRuleRequest) (*rule.Rule, error) {
	if req.RuleCode == "" || req.RuleName == "" {
		return nil, errs.New(errs.CodeInvalidParams, "rule_code and rule_name required", 200)
	}
	if len(req.Allocations) == 0 {
		return nil, errs.New(errs.CodeInvalidParams, "at least one allocation required", 200)
	}
	condJSON, err := json.Marshal(req.Conditions)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInvalidParams, "invalid conditions", 200, err)
	}
	allocJSON, err := json.Marshal(req.Allocations)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInvalidParams, "invalid allocations", 200, err)
	}
	status := req.Status
	if status == 0 {
		status = 1
	}
	m := &repository.SplitRuleModel{
		RuleCode:     req.RuleCode,
		MerchantID:   merchantID,
		RuleName:     req.RuleName,
		Priority:     req.Priority,
		Conditions:   string(condJSON),
		Allocations:  string(allocJSON),
		TriggerType:  "PAID",
		Status:       status,
	}
	if err := s.ruleRepo.Create(ctx, m); err != nil {
		return nil, errs.Wrap(errs.CodeInternalError, "create split rule failed", 200, err)
	}
	return s.ruleRepo.GetByCodeAndMerchant(ctx, req.RuleCode, merchantID)
}

// UpdateRule 更新分账规则（仅更新提供字段）。
func (s *Service) UpdateRule(ctx context.Context, id, merchantID uint64, req *UpdateRuleRequest) (*rule.Rule, error) {
	exist, err := s.ruleRepo.GetByID(ctx, id, merchantID)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternalError, "query split rule failed", 200, err)
	}
	if exist == nil {
		return nil, errs.New(errs.CodeInvalidParams, "split rule not found", 200)
	}
	fields := map[string]any{}
	if req.RuleName != nil {
		fields["rule_name"] = *req.RuleName
	}
	if req.Priority != nil {
		fields["priority"] = *req.Priority
	}
	if req.Conditions != nil {
		condJSON, err := json.Marshal(req.Conditions)
		if err != nil {
			return nil, errs.Wrap(errs.CodeInvalidParams, "invalid conditions", 200, err)
		}
		fields["conditions"] = string(condJSON)
	}
	if req.Allocations != nil {
		if len(*req.Allocations) == 0 {
			return nil, errs.New(errs.CodeInvalidParams, "at least one allocation required", 200)
		}
		allocJSON, err := json.Marshal(req.Allocations)
		if err != nil {
			return nil, errs.Wrap(errs.CodeInvalidParams, "invalid allocations", 200, err)
		}
		fields["allocations"] = string(allocJSON)
	}
	if len(fields) == 0 {
		return nil, errs.New(errs.CodeInvalidParams, "nothing to update", 200)
	}
	if err := s.ruleRepo.Update(ctx, id, merchantID, fields); err != nil {
		return nil, errs.Wrap(errs.CodeInternalError, "update split rule failed", 200, err)
	}
	return s.ruleRepo.GetByCodeAndMerchant(ctx, exist.RuleCode, merchantID)
}

// SetRuleStatus 更新规则启用状态（1 启用 / 0 停用）。
func (s *Service) SetRuleStatus(ctx context.Context, id, merchantID uint64, status int) error {
	exist, err := s.ruleRepo.GetByID(ctx, id, merchantID)
	if err != nil {
		return errs.Wrap(errs.CodeInternalError, "query split rule failed", 200, err)
	}
	if exist == nil {
		return errs.New(errs.CodeInvalidParams, "split rule not found", 200)
	}
	return s.ruleRepo.UpdateStatus(ctx, id, merchantID, status)
}

// DeleteRule 删除分账规则。
func (s *Service) DeleteRule(ctx context.Context, id, merchantID uint64) error {
	exist, err := s.ruleRepo.GetByID(ctx, id, merchantID)
	if err != nil {
		return errs.Wrap(errs.CodeInternalError, "query split rule failed", 200, err)
	}
	if exist == nil {
		return errs.New(errs.CodeInvalidParams, "split rule not found", 200)
	}
	return s.ruleRepo.Delete(ctx, id, merchantID)
}