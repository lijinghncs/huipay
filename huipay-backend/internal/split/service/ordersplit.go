package service

import (
	"context"
	"errors"
	
	"time"

	"github.com/huipay/huipay-backend/infra/errs"
	"github.com/huipay/huipay-backend/infra/prom"
	"github.com/huipay/huipay-backend/internal/domain/vo"
	"github.com/huipay/huipay-backend/internal/split/alloc"
	"github.com/huipay/huipay-backend/internal/split/executor"
	"github.com/huipay/huipay-backend/internal/split/repository"
	"github.com/huipay/huipay-backend/internal/split/rule"
	"github.com/huipay/huipay-backend/internal/split/state"
)

// ExecuteRequest 分账执行请求（HTTP 层）。
type ExecuteRequest struct {
	OrderNo    string         `json:"order_no" binding:"required"`
	MerchantID uint64         `json:"merchant_id"` // 由 handler 从登录上下文填充，忽略请求体
	Amount     int64          `json:"amount" binding:"required,gt=0"`
	RuleCode   string         `json:"rule_code"` // 可选：指定规则
	StoreID    uint64         `json:"store_id"`
	Channel    vo.ChannelCode `json:"channel"`
	PaidAt     string         `json:"paid_at"` // RFC3339，订单支付时间（全门店分摊按此时间截取实收）
	TraceID    string         `json:"-"`
}

// ExecuteResponse 分账执行响应。
type ExecuteResponse struct {
	OrderNo       string                `json:"order_no"`
	Allocations   []executor.Allocation `json:"allocations"`
	RuleCode      string                `json:"rule_code"`
	Status        string                `json:"status"`
	DegradedCount int                   `json:"degraded_count"`
}

// Execute 执行分账：按规则引擎匹配（支持门店维度），解析分配方案后落地账本。
func (s *Service) Execute(ctx context.Context, req *ExecuteRequest) (*ExecuteResponse, error) {
	if s.ruleRepo == nil {
		return nil, errs.New(errs.CodeInternalError, "split rule repo not configured", 500)
	}
	if s.ruleEngine == nil || s.executor == nil {
		return nil, errs.New(errs.CodeInternalError, "split engine not configured", 500)
	}

	// 匹配规则
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

	// 计算分配方案
	paidAt := alloc.ParsePaidAt(req.PaidAt)
	revenues, _ := s.revenueQuerier.SumPaidByStore(ctx, req.MerchantID, time.Time{}, paidAt)
	allocations, aErr := alloc.Compute(alloc.Input{
		Rule:          matched,
		Total:         req.Amount,
		StoreRevenues: toAllocRevenues(revenues),
	})
	if aErr != nil {
		return nil, aErr
	}

	// 源账户：商户钱包
	merchantWalletID, wErr := s.walletResolver.GetWalletByEntityType(ctx, req.MerchantID, vo.EntityMerchant)
	if wErr != nil {
		return nil, errs.New(errs.CodeInternalError, "merchant wallet not found", 200)
	}

	// 执行落地
	if err := s.executor.Execute(ctx, &executor.ExecuteRequest{
		MerchantID:     req.MerchantID,
		OrderNo:        req.OrderNo,
		SourceWallet:   merchantWalletID,
		Allocations:    allocations,
		StoreID:        req.StoreID,
		RuleID:         matched.ID,
		Channel:        req.Channel,
		IdempotencyKey: "split",
		TraceID:        req.TraceID,
	}); err != nil {
		return nil, errs.Wrap(errs.CodeInternalError, "split execute failed", 200, err)
	}
	return &ExecuteResponse{OrderNo: req.OrderNo, Allocations: allocations, RuleCode: matched.RuleCode}, nil
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

// ExecutionFilter 分账记录过滤（HTTP 层）。
type ExecutionFilter struct {
	Status string
	Start  string // RFC3339（可选）
	End    string // RFC3339（可选）
	RuleID uint64
}

// ListExecutions 分页查询商户分账记录（按订单聚合，支持状态/时间/规则过滤）。
func (s *Service) ListExecutions(ctx context.Context, merchantID uint64, page, size int, f ExecutionFilter) (*ExecutionPage, error) {
	if s.executor == nil {
		return nil, errs.New(errs.CodeInternalError, "split executor not configured", 500)
	}
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 200 {
		size = 20
	}
	ef := executor.SplitExecutionFilter{Status: f.Status, RuleID: f.RuleID}
	if f.Start != "" {
		if t, err := time.Parse(time.RFC3339, f.Start); err == nil {
			ef.Start = t
		}
	}
	if f.End != "" {
		if t, err := time.Parse(time.RFC3339, f.End); err == nil {
			ef.End = t
		}
	}
	items, total, err := s.executor.ListByMerchant(ctx, merchantID, (page-1)*size, size, ef)
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

// PreviewRequest 分账预览请求（不落库，仅试算）。
// 两种模式二选一：Amount（单笔试算）或 Start/End（时间段账单预览）。
type PreviewRequest struct {
	RuleCode string   `json:"rule_code" binding:"required"`
	Amount   int64    `json:"amount"`     // 单笔试算金额（分），>0 时启用单笔模式
	Start    string   `json:"start"`      // 时间段模式起始（RFC3339）
	End      string   `json:"end"`        // 时间段模式结束（RFC3339）
	StoreIDs []uint64 `json:"store_ids"`  // 可选：限定参与分摊的门店（空=全部门店）
	Channel  string   `json:"channel"`    // 可选：通道
}

// PreviewItem 预览明细行。
type PreviewItem struct {
	ReceiverEntityID uint64 `json:"receiver_entity_id"`
	ReceiverType     string `json:"receiver_type"`
	ReceiverName     string `json:"receiver_name"`
	Amount           int64  `json:"amount"`
	Ratio            int64  `json:"ratio"` // 万分比（金额 ÷ 总额 * 10000）
}

// PreviewResponse 分账预览响应。
type PreviewResponse struct {
	RuleCode       string        `json:"rule_code"`
	RuleName       string        `json:"rule_name"`
	Mode           string        `json:"mode"` // amount / period
	TotalAmount    int64         `json:"total_amount"`
	Items          []PreviewItem `json:"items"`
	MerchantRemain int64         `json:"merchant_remain"` // 未分配归商户金额（分）
}

// Preview 分账试算：选定规则，按单笔金额或时间段实收预览分配，不落库。
func (s *Service) Preview(ctx context.Context, merchantID uint64, req *PreviewRequest) (*PreviewResponse, error) {
	if s.ruleRepo == nil {
		return nil, errs.New(errs.CodeInternalError, "split rule repo not configured", 500)
	}
	matched, err := s.ruleRepo.GetByCodeAndMerchant(ctx, req.RuleCode, merchantID)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternalError, "query split rule failed", 200, err)
	}
	if matched == nil {
		return nil, errs.New(errs.CodeSplitRuleNotMatch, "split rule not found", 200)
	}

	var allocations []executor.Allocation
	var total int64
	var mode string
	switch {
	case req.Amount > 0:
		mode = "amount"
		total = req.Amount
		allocations, err = s.previewAlloc(ctx, matched, total, time.Time{}, req.StoreIDs)
		if err != nil {
			return nil, err
		}
	case req.Start != "" && req.End != "":
		mode = "period"
		start, err1 := time.Parse(time.RFC3339, req.Start)
		end, err2 := time.Parse(time.RFC3339, req.End)
		if err1 != nil || err2 != nil {
			return nil, errs.New(errs.CodeInvalidParams, "invalid start/end time", 200)
		}
		if !end.After(start) {
			return nil, errs.New(errs.CodeInvalidParams, "end must be after start", 200)
		}
		if s.revenueQuerier == nil {
			return nil, errs.New(errs.CodeInternalError, "store revenue querier not configured", 500)
		}
		total, err = s.revenueQuerier.SumPaid(ctx, merchantID, start, end)
		if err != nil {
			return nil, errs.Wrap(errs.CodeInternalError, "query paid total failed", 200, err)
		}
		if total <= 0 {
			return nil, errs.New(errs.CodeInvalidParams, "所选时间段内没有实收金额", 200)
		}
		allocations, err = s.previewAllocPeriod(ctx, matched, total, start, end, req.StoreIDs)
		if err != nil {
			return nil, err
		}
	default:
		return nil, errs.New(errs.CodeInvalidParams, "amount or start/end required", 200)
	}

	items := toSplitBillItems(allocations)
	if err := s.fillBillItemNames(ctx, items); err != nil {
		return nil, errs.Wrap(errs.CodeInternalError, "fill receiver names failed", 200, err)
	}
	out := make([]PreviewItem, 0, len(items))
	var used int64
	for _, it := range items {
		ratio := int64(0)
		if total > 0 {
			ratio = it.Amount * 10000 / total
		}
		used += it.Amount
		out = append(out, PreviewItem{
			ReceiverEntityID: it.ReceiverEntityID,
			ReceiverType:     it.ReceiverType,
			ReceiverName:     it.ReceiverName,
			Amount:           it.Amount,
			Ratio:            ratio,
		})
	}
	remain := total - used
	if remain < 0 {
		remain = 0
	}
	return &PreviewResponse{
		RuleCode:       matched.RuleCode,
		RuleName:       matched.RuleName,
		Mode:           mode,
		TotalAmount:    total,
		Items:          out,
		MerchantRemain: remain,
	}, nil
}

// previewAlloc 单笔金额试算分配。
func (s *Service) previewAlloc(ctx context.Context, r *rule.Rule, total int64, paidAt time.Time, storeIDs []uint64) ([]executor.Allocation, error) {
	revenues, _ := s.revenueQuerier.SumPaidByStore(ctx, r.MerchantID, time.Time{}, paidAt)
	return alloc.Compute(alloc.Input{
		Rule:           r,
		Total:          total,
		StoreRevenues:  toAllocRevenues(revenues),
		FilterStoreIDs: storeIDs,
	})
}

// previewAllocPeriod 时间段试算分配。
func (s *Service) previewAllocPeriod(ctx context.Context, r *rule.Rule, total int64, start, end time.Time, storeIDs []uint64) ([]executor.Allocation, error) {
	revenues, _ := s.revenueQuerier.SumPaidByStore(ctx, r.MerchantID, start, end)
	return alloc.Compute(alloc.Input{
		Rule:           r,
		Total:          total,
		StoreRevenues:  toAllocRevenues(revenues),
		FilterStoreIDs: storeIDs,
	})
}

// RetryResult 重试结果。
type RetryResult struct {
	OrderNo string `json:"order_no"`
	Success int    `json:"success"` // 重试后成功接收方数
	Failed  int    `json:"failed"`  // 重试后失败接收方数
	Retried int    `json:"retried"` // 本次实际重试的接收方数
}

// RetryExecution 重试失败/部分失败的订单分账：仅重建未成功接收方的分配并重跑 executor（幂等跳过已成功接收方）。
func (s *Service) RetryExecution(ctx context.Context, merchantID uint64, orderNo string) (*RetryResult, error) {
	if s.executor == nil {
		return nil, errs.New(errs.CodeInternalError, "split executor not configured", 500)
	}
	rows, err := s.executor.ListByOrderNoForMerchant(ctx, merchantID, orderNo)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternalError, "query split execution failed", 200, err)
	}
	if rows == nil {
		return nil, errs.New(errs.CodeInvalidParams, "split execution not found", 200)
	}
	if len(rows) == 0 {
		return nil, errs.New(errs.CodeInvalidParams, "no split execution to retry", 200)
	}

	allocations := make([]executor.Allocation, 0, len(rows))
	var ruleID, storeID uint64
	var ch vo.ChannelCode
	for _, r := range rows {
		if r.Status == "SUCCESS" {
			continue
		}
		allocations = append(allocations, executor.Allocation{
			Level:      r.Level,
			EntityID:   r.ReceiverEntityID,
			EntityType: vo.EntityType(r.ReceiverType),
			Amount:     r.Amount,
		})
		if r.RuleID != nil {
			ruleID = *r.RuleID
		}
		if r.StoreID != nil {
			storeID = *r.StoreID
		}
		ch = vo.ChannelCode(r.Channel)
	}
	if len(allocations) == 0 {
		return &RetryResult{OrderNo: orderNo, Success: len(rows), Failed: 0, Retried: 0}, nil
	}

	merchantWalletID, wErr := s.walletResolver.GetWalletByEntityType(ctx, merchantID, vo.EntityMerchant)
	if wErr != nil {
		return nil, errs.New(errs.CodeInternalError, "merchant wallet not found", 200)
	}

	if err := s.executor.Execute(ctx, &executor.ExecuteRequest{
		MerchantID:     merchantID,
		OrderNo:        orderNo,
		SourceWallet:   merchantWalletID,
		Allocations:    allocations,
		StoreID:        storeID,
		RuleID:         ruleID,
		Channel:        ch,
		IdempotencyKey: "split",
		TraceID:        "",
	}); err != nil {
		return nil, errs.Wrap(errs.CodeInternalError, "retry split execution failed", 200, err)
	}
	s.appendAudit(ctx, "EXECUTION", orderNo, "RETRY", merchantID, map[string]any{"retried": len(allocations)})
	prom.SplitRetryTotal.Inc()

	after, err := s.executor.ListByOrderNo(ctx, orderNo)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternalError, "query split execution failed", 200, err)
	}
	var success, failed int
	for _, r := range after {
		if r.Status == "SUCCESS" {
			success++
		} else {
			failed++
		}
	}
	return &RetryResult{OrderNo: orderNo, Success: success, Failed: failed, Retried: len(allocations)}, nil
}

// ReopenExecution 死单复位重开：DEAD → FAILED 并清零重试计数，交由补偿调度自动重入。
func (s *Service) ReopenExecution(ctx context.Context, merchantID uint64, orderNo string) error {
	if s.orderStatusRepo == nil {
		return errs.New(errs.CodeInternalError, "split order status repo not configured", 500)
	}
	st, err := s.orderStatusRepo.Get(ctx, orderNo)
	if err != nil {
		return errs.Wrap(errs.CodeInternalError, "query split order status failed", 200, err)
	}
	if st == nil || st.MerchantID != merchantID {
		return errs.New(errs.CodeInvalidParams, "split execution not found", 200)
	}
	if st.Status != string(state.Dead) {
		return errs.New(errs.CodeInvalidParams, "仅重试耗尽（DEAD）的订单可复位重开", 200)
	}
	ok, err := s.orderStatusRepo.Reopen(ctx, orderNo)
	if err != nil {
		return errs.Wrap(errs.CodeInternalError, "reopen split execution failed", 200, err)
	}
	if !ok {
		return errs.New(errs.CodeInvalidParams, "订单状态已变化，请刷新后重试", 200)
	}
	s.appendAudit(ctx, repository.AuditBizTypeSplitExec, orderNo, repository.AuditActionReopen, merchantID, nil)
	return nil
}

// toAllocRevenues 将 repository 的 StoreRevenue 转为 alloc 包定义的类型。
func toAllocRevenues(in []repository.StoreRevenue) []alloc.StoreRevenue {
	if in == nil {
		return nil
	}
	out := make([]alloc.StoreRevenue, len(in))
	for i, r := range in {
		out[i] = alloc.StoreRevenue{StoreID: r.StoreID, Paid: r.Paid}
	}
	return out
}

// toSplitBillItems 将 executor.Allocation 切片转为 repository.SplitBillItem 切片。
func toSplitBillItems(allocations []executor.Allocation) []repository.SplitBillItem {
	items := make([]repository.SplitBillItem, 0, len(allocations))
	for _, a := range allocations {
		items = append(items, repository.SplitBillItem{
			ReceiverEntityID: a.EntityID,
			ReceiverType:     string(a.EntityType),
			Amount:           a.Amount,
		})
	}
	return items
}