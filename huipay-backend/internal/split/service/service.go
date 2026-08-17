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
	"github.com/huipay/huipay-backend/infra/prom"
	"github.com/huipay/huipay-backend/internal/account/service"
	"github.com/huipay/huipay-backend/internal/domain/vo"
	"github.com/huipay/huipay-backend/internal/split/executor"
	"github.com/huipay/huipay-backend/internal/split/recon"
	"github.com/huipay/huipay-backend/internal/split/repository"
	"github.com/huipay/huipay-backend/internal/split/rule"
	statsservice "github.com/huipay/huipay-backend/internal/stats/service"
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
	ruleEngine       *rule.Engine
	executor         *executor.Executor
	ruleRepo         *repository.SplitRuleRepo
	billRepo         *repository.SplitBillRepo
	billBizDateRepo  *repository.BillBizDateRepo
	dailyExecRepo    *repository.DailyExecutionRepo
	diffRepo         *repository.ReconcileDiffRepo
	auditRepo        *repository.SplitAuditRepo
	orderStatusRepo  *repository.SplitOrderStatusRepo
	account          *service.Service
	revenueQuerier   repository.StoreRevenueQuerier
	prechecker       *recon.Prechecker
	statsSvc         *statsservice.Service
	logger           *zap.Logger
}

// NewService 构造 Service。
func NewService(
	re *rule.Engine, ex *executor.Executor,
	ruleRepo *repository.SplitRuleRepo,
	billRepo *repository.SplitBillRepo,
	billBizDateRepo *repository.BillBizDateRepo,
	dailyExecRepo *repository.DailyExecutionRepo,
	diffRepo *repository.ReconcileDiffRepo,
	auditRepo *repository.SplitAuditRepo,
	orderStatusRepo *repository.SplitOrderStatusRepo,
	acc *service.Service,
	revQuerier repository.StoreRevenueQuerier,
	prechecker *recon.Prechecker,
	statsSvc *statsservice.Service,
	logger *zap.Logger,
) *Service {
	return &Service{
		ruleEngine: re, executor: ex,
		ruleRepo: ruleRepo, billRepo: billRepo,
		billBizDateRepo: billBizDateRepo,
		dailyExecRepo:   dailyExecRepo,
		diffRepo:        diffRepo,
		auditRepo:       auditRepo,
		orderStatusRepo: orderStatusRepo,
		account:         acc, revenueQuerier: revQuerier,
		prechecker: prechecker, statsSvc: statsSvc,
		logger: logger,
	}
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
		MerchantID:    req.MerchantID,
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

// ExecuteByPeriod 按时间段分账：V2 合并版全流程（前置对账 + 每日执行 + 状态机补偿）。
//
// 流程：
//  1. 解析时段 + 选定规则
//  2. 幂等短路：billRepo.GetByBatchNo 命中 → 直接返回
//  3. 跨规则重复防护：billBizDateRepo.ListBillsByDate 命中时段内已有账单 → 拒绝
//  4. Prechecker 双层对账（自动 Backfill + Layer A + Layer B）
//  5. daily_execution_repo.CreateWithRunID(RUNNING)
//  6. 计算 total + buildAllocationsPeriod
//  7. executor.Execute (内部自治，不外层包事务)
//  8. 反查 t_split_execution 事实 → daily_execution.MarkStatus
//  9. billRepo.Create + billBizDateRepo.Bind + 回填订单 split_batch_no
//  10. 触发 async recompute（同步调用，迭代 2 改 outbox）
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

	// 批次号：确定性（同一时间段+规则幂等），供分账记录聚合展示
	batchNo := fmt.Sprintf("SP%d-%d-%d", matched.ID, start.Unix(), end.Unix())

	// 2. 幂等短路（V2 评审要点 🔴4 / 🟠14）
	if s.billRepo != nil {
		if existing, _ := s.billRepo.GetByBatchNo(ctx, batchNo, merchantID); existing != nil {
			return &ExecuteByPeriodResponse{
				BatchNo: existing.BatchNo,
				RuleCode: existing.RuleCode,
			}, nil
		}
	}

	// 3. 跨规则重复防护（V2 评审要点 🔴3）：时段内已有非 REJECTED 账单则拒绝
	if s.billBizDateRepo != nil {
		// 遍历时段内每个 biz_date 检查是否有任何账单覆盖（不论 EXECUTED 还是 PENDING/APPROVED）
		for d := start; d.Before(end); d = d.AddDate(0, 0, 1) {
			billIDs, _ := s.billBizDateRepo.ListBillsByDate(ctx, merchantID, d)
			if len(billIDs) > 0 {
				return nil, errs.New(
					errs.CodeSplitPeriodOverlapped,
					fmt.Sprintf("时段内 %s 已有账单覆盖，请先驳回或重跑", d.Format("2006-01-02")),
					200,
				)
			}
		}
	}

	// 4. Prechecker 双层对账（V2 评审要点 🔴1/2 + 性能 🔴7 已用 LEFT JOIN 优化）
	if s.prechecker != nil {
		result, pErr := s.prechecker.Check(ctx, merchantID, start, end)
		if pErr != nil {
			// 写失败执行记录 + 审计
			s.recordFailedDailyExec(ctx, merchantID, start, batchNo, pErr, nil)
			return nil, pErr
		}
		_ = result // 通过后继续
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
		return nil, errs.New(errs.CodeInvalidParams, "所选时间段内没有实收金额", 200)
	}
	// 记录时间段内尚未分账的订单号，供后续分账排除已分账订单（避免重复分账）
	orderNos, err := s.revenueQuerier.ListUnsplitOrderNos(ctx, merchantID, start, end)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternalError, "query unsplit orders failed", 200, err)
	}
	orderNosJSON, err := json.Marshal(orderNos)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInvalidParams, "marshal order list failed", 200, err)
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

	// 5. 创建每日执行记录（RUNNING）
	startedAt := time.Now()
	var runID string
	var dailyExecID uint64
	if s.dailyExecRepo != nil {
		bizDate := start
		runID = fmt.Sprintf("SP_RUN-%d-%s-%d", merchantID, batchNo, startedAt.UnixNano())
		m := &repository.DailyExecutionModel{
			RunID:      runID,
			MerchantID: merchantID,
			BizDate:    bizDate,
			BatchNo:    batchNo,
			Status:     repository.DailyExecRunning,
		}
		created, cErr := s.dailyExecRepo.CreateWithRunID(ctx, m)
		if cErr != nil {
			s.logger.Warn("create daily exec fail", zap.String("run_id", runID), zap.Error(cErr))
		} else if created != nil {
			dailyExecID = created.ID
		}
	}

	// 6. executor.Execute（内部自治，不外层包事务）
	execErr := s.executor.Execute(ctx, &executor.ExecuteRequest{
		MerchantID:     merchantID,
		OrderNo:        batchNo,
		SourceWallet:   merchantWallet.ID,
		Allocations:    allocations,
		RuleID:         matched.ID,
		IdempotencyKey: "split",
		TraceID:        "",
	})

	// 7. 反查事实（V2 评审要点 🔴4）：从 t_split_execution 真实状态判定终态
	durationMs := int(time.Since(startedAt) / time.Millisecond)
	var finalStatus string
	var errCode, errMsg string
	var diffID *uint64
	if execErr != nil {
		finalStatus = repository.DailyExecFailed
		errCode = errs.CodeInternalError
		errMsg = truncateMsg(execErr.Error())
	} else {
		// 读事实：分账明细
		receivers, rErr := s.readExecutionFacts(ctx, batchNo)
		if rErr != nil {
			finalStatus = repository.DailyExecFailed
			errCode = errs.CodeInternalError
			errMsg = truncateMsg(rErr.Error())
		} else {
			success, total2 := countReceivers(receivers)
			switch {
			case total2 == 0:
				finalStatus = repository.DailyExecFailed
				errCode = errs.CodeInternalError
				errMsg = "no split executions"
			case success == total2:
				finalStatus = repository.DailyExecSuccess
			case success == 0:
				finalStatus = repository.DailyExecFailed
			default:
				finalStatus = repository.DailyExecPartial
			}
		}
	}

	// 8. 更新每日执行记录
	if s.dailyExecRepo != nil && dailyExecID > 0 {
		_ = s.dailyExecRepo.MarkStatus(ctx, dailyExecID, finalStatus, errCode, errMsg, diffID, durationMs)
	}
	prom.SplitDailyExecTotal.WithLabelValues(finalStatus).Inc()
	prom.SplitDailyExecDuration.Observe(float64(durationMs))

	// 9. 审计
	if s.auditRepo != nil {
		action := repository.AuditActionExecute
		if finalStatus != repository.DailyExecSuccess {
			action = repository.AuditActionExecuteFailed
		}
		_ = s.auditRepo.WriteAction(ctx, repository.AuditBizTypeDailySplit, batchNo, action,
			repository.AuditOperatorSystem, 0,
			map[string]any{"status": finalStatus, "duration_ms": durationMs, "error": errMsg})
	}

	if execErr != nil {
		return nil, errs.Wrap(errs.CodeInternalError, "split execute failed", 200, execErr)
	}

	// 10. 落地分账单 + 关联表
	now := time.Now()
	bizDates := collectBizDates(start, end)
	detailJSON, _ := json.Marshal(allocationsToItems(allocations))
	bizDatesJSON, _ := json.Marshal(bizDates)
	executedBill := &repository.SplitBillModel{
		BatchNo:     batchNo,
		MerchantID:  merchantID,
		RuleCode:    matched.RuleCode,
		RuleName:    matched.RuleName,
		StartTime:   start,
		EndTime:     end,
		TotalAmount: total,
		Detail:      string(detailJSON),
		OrderNos:    string(orderNosJSON),
		BizDates:    string(bizDatesJSON),
		Status:      repository.BillExecuted,
		ApprovedAt:  &now,
		ExecutedAt:  &now,
	}
	billID := uint64(0)
	if err := s.billRepo.Create(ctx, executedBill); err != nil {
		s.logger.Warn("record executed split bill failed",
			zap.String("batch_no", batchNo), zap.Error(err))
	} else {
		billID = executedBill.ID
	}
	if s.billBizDateRepo != nil && billID > 0 {
		if err := s.billBizDateRepo.Bind(ctx, billID, bizDates); err != nil {
			s.logger.Warn("bind bill biz date fail",
				zap.String("batch_no", batchNo), zap.Error(err))
		}
	}

	// 11. 回填所属批次号，供门店订单明细按批次号关联查询
	if err := s.fillSplitBatchNo(ctx, merchantID, batchNo, orderNos); err != nil {
		s.logger.Warn("backfill split batch no fail",
			zap.String("batch_no", batchNo), zap.Error(err))
	}
	// 11.1 批次已执行，统一回写订单分账状态为 SUCCESS（保证交易明细口径一致）
	if err := s.billRepo.MarkOrdersSplit(ctx, merchantID, batchNo); err != nil {
		s.logger.Warn("mark orders split fail",
			zap.String("batch_no", batchNo), zap.Error(err))
	}

	// 12. 触发异步汇总：每个 biz_date 跑一次（V2 评审要点 🔴6）
	if s.statsSvc != nil {
		go func() {
			for _, bd := range bizDates {
				if _, err := s.statsSvc.RecomputeSplitStatus(context.Background(), merchantID, bd); err != nil {
					s.logger.Warn("async recompute split status fail",
						zap.Uint64("merchant", merchantID),
						zap.Time("biz_date", bd),
						zap.Error(err))
				}
			}
		}()
	}

	return &ExecuteByPeriodResponse{
		BatchNo:     batchNo,
		TotalAmount: total,
		RuleCode:    matched.RuleCode,
		Allocations: allocations,
	}, nil
}

// readExecutionFacts 反查 t_split_execution 真实状态（V2 评审要点 🔴4：避免扩展 Executor 返回契约）。
func (s *Service) readExecutionFacts(ctx context.Context, orderNo string) ([]SplitExecutionRow, error) {
	type row struct {
		Status string `gorm:"column:status"`
	}
	var rows []row
	q := `SELECT status FROM t_split_execution WHERE order_no = ?`
	if err := s.billRepo.DB().WithContext(ctx).Raw(q, orderNo).Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]SplitExecutionRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, SplitExecutionRow{Status: r.Status})
	}
	return out, nil
}

// SplitExecutionRow 反查辅助结构。
type SplitExecutionRow struct {
	Status string
}

func countReceivers(rows []SplitExecutionRow) (success, total int) {
	total = len(rows)
	for _, r := range rows {
		if r.Status == "SUCCESS" {
			success++
		}
	}
	return
}

// recordFailedDailyExec Prechecker 失败时写每日执行记录。
func (s *Service) recordFailedDailyExec(ctx context.Context, merchantID uint64, bizDate time.Time, batchNo string, cause error, diffID *uint64) {
	if s.dailyExecRepo == nil {
		return
	}
	bizCode := ""
	msg := ""
	if cause != nil {
		bizCode = "RECONCILE_FAILED"
		msg = truncateMsg(cause.Error())
	}
	runID := fmt.Sprintf("SP_RUN-FAIL-%d-%s-%d", merchantID, batchNo, time.Now().UnixNano())
	m := &repository.DailyExecutionModel{
		RunID:           runID,
		MerchantID:      merchantID,
		BizDate:         bizDate,
		BatchNo:         batchNo,
		Status:          repository.DailyExecFailed,
		ErrorCode:       &bizCode,
		ErrorMessage:    &msg,
		ReconcileDiffID: diffID,
	}
	if created, err := s.dailyExecRepo.CreateWithRunID(ctx, m); err == nil && created != nil {
		_ = s.dailyExecRepo.MarkStatus(ctx, created.ID, repository.DailyExecFailed, bizCode, msg, diffID, 0)
	}
}

// collectBizDates 收集 [start, end) 区间内每个自然日（按本地时区）。
func collectBizDates(start, end time.Time) []time.Time {
	out := []time.Time{}
	for d := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.Local); d.Before(end); d = d.AddDate(0, 0, 1) {
		out = append(out, d)
	}
	return out
}

// truncateMsg 截断错误信息至 1000 字符（V2 评审要点 🟡20）。
func truncateMsg(s string) string {
	const max = 1000
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
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
			expanded, err := s.expandAllStores(ctx, r.MerchantID, amount, i+1, start, end, nil)
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
		return nil, errs.New(errs.CodeInvalidParams, "所选时间段内没有实收金额", 200)
	}
	// 记录时间段内尚未分账的订单号，供后续分账排除已分账订单（避免重复分账）
	orderNos, err := s.revenueQuerier.ListUnsplitOrderNos(ctx, merchantID, start, end)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternalError, "query unsplit orders failed", 200, err)
	}
	orderNosJSON, err := json.Marshal(orderNos)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInvalidParams, "marshal order list failed", 200, err)
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
	bizDates := collectBizDates(start, end)
	bizDatesJSON, _ := json.Marshal(bizDates)
	m := &repository.SplitBillModel{
		BatchNo:     batchNo,
		MerchantID:  merchantID,
		RuleCode:    matched.RuleCode,
		RuleName:    matched.RuleName,
		StartTime:   start,
		EndTime:     end,
		TotalAmount: total,
		Detail:      string(detailJSON),
		OrderNos:    string(orderNosJSON),
		BizDates:    string(bizDatesJSON),
		Status:      repository.BillPending,
	}
	if err := s.billRepo.Create(ctx, m); err != nil {
		s.logger.Error("create split bill failed",
			zap.String("batch_no", batchNo), zap.Error(err))
		return nil, errs.Wrap(errs.CodeInternalError, "create split bill failed", 200, err)
	}
	// 绑定账单覆盖的业务日期（供按日过滤/排除已分账订单）
	if s.billBizDateRepo != nil {
		_ = s.billBizDateRepo.Bind(ctx, m.ID, bizDates)
	}
	// 回填所属批次号，供门店订单明细按批次号关联查询
	if err := s.fillSplitBatchNo(ctx, merchantID, batchNo, orderNos); err != nil {
		s.logger.Warn("backfill split batch no fail",
			zap.String("batch_no", batchNo), zap.Error(err))
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

// BillStoreItem 批次内门店汇总行。
type BillStoreItem struct {
	StoreID   uint64 `json:"store_id"`
	StoreName string `json:"store_name"`
	Amount    int64  `json:"amount"`
	Ratio     string `json:"ratio"` // 占比百分比（两位小数）
}

// BillStoreSummary 批次门店汇总视图。
type BillStoreSummary struct {
	BatchNo     string          `json:"batch_no"`
	RuleCode    string          `json:"rule_code"`
	RuleName    string          `json:"rule_name"`
	StartTime   string          `json:"start_time"`
	EndTime     string          `json:"end_time"`
	TotalAmount int64           `json:"total_amount"`
	Status      string          `json:"status"`
	Stores      []BillStoreItem `json:"stores"`
}

// BillStoreOrder 批次内某门店的订单行。
type BillStoreOrder struct {
	OrderNo string  `json:"order_no"`
	Amount  int64   `json:"amount"`
	Status  string  `json:"status"`
	PaidAt  *string `json:"paid_at"`
}

// BillStoreOrders 批次内某门店订单明细视图。
type BillStoreOrders struct {
	BatchNo   string           `json:"batch_no"`
	StoreID   uint64           `json:"store_id"`
	StoreName string           `json:"store_name"`
	Orders    []BillStoreOrder `json:"orders"`
}

// BillStoreSummary 查询某分账批次号下的门店汇总（门店名 / 可分金额 / 占比）。
func (s *Service) BillStoreSummary(ctx context.Context, merchantID uint64, batchNo string) (*BillStoreSummary, error) {
	if s.billRepo == nil {
		return nil, errs.New(errs.CodeInternalError, "split bill repo not configured", 500)
	}
	bill, err := s.billRepo.GetByBatchNo(ctx, batchNo, merchantID)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternalError, "query split bill failed", 200, err)
	}
	if bill == nil || bill.Status == repository.BillRejected {
		return nil, errs.New(errs.CodeInvalidParams, "split bill not found", 200)
	}
	var items []repository.SplitBillItem
	_ = json.Unmarshal([]byte(bill.Detail), &items)
	// 仅取门店(STORE)接收方
	var storeIDs []uint64
	amountByStore := make(map[uint64]int64)
	for _, it := range items {
		if it.ReceiverType != string(vo.EntityStore) {
			continue
		}
		storeIDs = append(storeIDs, it.ReceiverEntityID)
		amountByStore[it.ReceiverEntityID] = it.Amount
	}
	names, err := s.billRepo.GetStoreNames(ctx, storeIDs)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternalError, "query store names failed", 200, err)
	}
	stores := make([]BillStoreItem, 0, len(storeIDs))
	for _, sid := range storeIDs {
		amount := amountByStore[sid]
		ratio := ""
		if bill.TotalAmount > 0 {
			ratio = fmt.Sprintf("%.2f", float64(amount)/float64(bill.TotalAmount)*100)
		}
		stores = append(stores, BillStoreItem{
			StoreID:   sid,
			StoreName: names[sid],
			Amount:    amount,
			Ratio:     ratio,
		})
	}
	return &BillStoreSummary{
		BatchNo:     bill.BatchNo,
		RuleCode:    bill.RuleCode,
		RuleName:    bill.RuleName,
		StartTime:   bill.StartTime.Format(time.RFC3339),
		EndTime:     bill.EndTime.Format(time.RFC3339),
		TotalAmount: bill.TotalAmount,
		Status:      bill.Status,
		Stores:      stores,
	}, nil
}

// fillSplitBatchNo 将分账批次号回填到覆盖订单（供「门店订单明细」按批次号关联查询）。
// 仅回填尚未归属批次(split_batch_no IS NULL)的订单，避免覆盖已分账订单。
func (s *Service) fillSplitBatchNo(ctx context.Context, merchantID uint64, batchNo string, orderNos []string) error {
	if s.billRepo == nil || len(orderNos) == 0 {
		return nil
	}
	return s.billRepo.DB().WithContext(ctx).Table("t_order").
		Where("merchant_id = ? AND order_no IN ? AND deleted_at IS NULL AND split_batch_no IS NULL", merchantID, orderNos).
		Update("split_batch_no", batchNo).Error
}

// releaseSplitBatchNo 驳回分账单时释放批次号（该批次订单可被后续重新分账）。
func (s *Service) releaseSplitBatchNo(ctx context.Context, merchantID uint64, batchNo string) error {
	if s.billRepo == nil {
		return nil
	}
	return s.billRepo.DB().WithContext(ctx).Table("t_order").
		Where("merchant_id = ? AND split_batch_no = ?", merchantID, batchNo).
		Update("split_batch_no", nil).Error
}

// BillStoreOrders 查询某分账批次号下某门店对应的订单交易明细。
func (s *Service) BillStoreOrders(ctx context.Context, merchantID uint64, batchNo string, storeID uint64) (*BillStoreOrders, error) {
	if s.billRepo == nil {
		return nil, errs.New(errs.CodeInternalError, "split bill repo not configured", 500)
	}
	if storeID == 0 {
		return nil, errs.New(errs.CodeInvalidParams, "store_id required", 200)
	}
	bill, err := s.billRepo.GetByBatchNo(ctx, batchNo, merchantID)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternalError, "query split bill failed", 200, err)
	}
	if bill == nil || bill.Status == repository.BillRejected {
		return nil, errs.New(errs.CodeInvalidParams, "split bill not found", 200)
	}
	storeName := ""
	if n, err := s.billRepo.GetStoreNames(ctx, []uint64{storeID}); err == nil {
		storeName = n[storeID]
	}
	type row struct {
		OrderNo string
		Amount  int64
		Status  string
		PaidAt  *time.Time
	}
	var rows []row
	// 按批次号 + 门店关联查询（走 idx_split_batch 索引），替代 order_no IN (order_nos) 大列表反查。
	if err := s.billRepo.DB().WithContext(ctx).Table("t_order").
		Select("order_no, amount, status, paid_at").
		Where("merchant_id = ? AND split_batch_no = ? AND store_id = ? AND deleted_at IS NULL", merchantID, batchNo, storeID).
		Order("created_at DESC").
		Scan(&rows).Error; err != nil {
		return nil, errs.Wrap(errs.CodeInternalError, "query store orders failed", 200, err)
	}
	orders := make([]BillStoreOrder, 0, len(rows))
	for _, r := range rows {
		var paidAt *string
		if r.PaidAt != nil {
			s := r.PaidAt.Format(time.RFC3339)
			paidAt = &s
		}
		orders = append(orders, BillStoreOrder{OrderNo: r.OrderNo, Amount: r.Amount, Status: r.Status, PaidAt: paidAt})
	}
	return &BillStoreOrders{
		BatchNo:   bill.BatchNo,
		StoreID:   storeID,
		StoreName: storeName,
		Orders:    orders,
	}, nil
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
		MerchantID:    merchantID,
		OrderNo:       bill.BatchNo,
		SourceWallet:  merchantWallet.ID,
		Allocations:   allocations,
		RuleID:        0,
		IdempotencyKey: "split",
		TraceID:       "",
	}); err != nil {
		return nil, errs.Wrap(errs.CodeInternalError, "split execute failed", 200, err)
	}

	now := time.Now()
	ok, uErr := s.billRepo.UpdateStatus(ctx, bill.ID, repository.BillExecuted, map[string]any{
		"approved_at": now,
		"executed_at": now,
	})
	if uErr != nil {
		return nil, errs.Wrap(errs.CodeInternalError, "update split bill failed", 200, uErr)
	}
	if !ok {
		// 乐观锁：账单已被并发审批/处理，资金只分一次（executor 幂等兜底）
		return nil, errs.New(errs.CodeInvalidParams, "bill already processed by another request", 200)
	}
	s.appendAudit(ctx, "BILL", batchNo, "APPROVE", merchantID, map[string]any{"total_amount": bill.TotalAmount})
	// 批次模式统一回写订单分账状态，保证交易明细口径一致
	if err := s.billRepo.MarkOrdersSplit(ctx, merchantID, batchNo); err != nil {
		s.logger.Warn("mark orders split fail",
			zap.String("batch_no", batchNo), zap.Error(err))
	}
	bill.Status = repository.BillExecuted
	bill.ApprovedAt = &now
	bill.ExecutedAt = &now
	return billToDTO(bill), nil
}

// RejectBill 驳回分账单（仅待审批状态可驳回，乐观锁防并发）。
func (s *Service) RejectBill(ctx context.Context, merchantID uint64, batchNo string) (*BillDTO, error) {
	bill, _, err := s.getPendingBillRaw(ctx, merchantID, batchNo)
	if err != nil {
		return nil, err
	}
	ok, uErr := s.billRepo.UpdateStatus(ctx, bill.ID, repository.BillRejected, nil)
	if uErr != nil {
		return nil, errs.Wrap(errs.CodeInternalError, "update split bill failed", 200, uErr)
	}
	if !ok {
		return nil, errs.New(errs.CodeInvalidParams, "bill already processed by another request", 200)
	}
	// 驳回后释放批次号，该批次覆盖订单可被后续重新分账
	if err := s.releaseSplitBatchNo(ctx, merchantID, batchNo); err != nil {
		s.logger.Warn("release split batch no fail",
			zap.String("batch_no", batchNo), zap.Error(err))
	}
	// 复位订单分账状态为 PENDING，保证交易明细口径一致
	if err := s.billRepo.ResetOrdersSplit(ctx, merchantID, batchNo); err != nil {
		s.logger.Warn("reset orders split fail",
			zap.String("batch_no", batchNo), zap.Error(err))
	}
	s.appendAudit(ctx, "BILL", batchNo, "REJECT", merchantID, nil)
	bill.Status = repository.BillRejected
	return billToDTO(bill), nil
}

// appendAudit 追加分账审计记录（D2；审计仓储未配置时静默忽略，不阻断业务流程）。
func (s *Service) appendAudit(ctx context.Context, bizType, bizID, action string, operatorID uint64, detail any) {
	if s.auditRepo == nil {
		return
	}
	var detailJSON *string // nil 写 NULL（JSON 列不接受空字符串，否则静默插入失败）
	if detail != nil {
		if b, err := json.Marshal(detail); err == nil {
			str := string(b)
			detailJSON = &str
		}
	}
	if err := s.auditRepo.Append(ctx, &repository.SplitAuditModel{
		BizType:      bizType,
		BizID:        bizID,
		Action:       action,
		OperatorType: "MERCHANT",
		OperatorID:   operatorID,
		Detail:       detailJSON,
	}); err != nil {
		s.logger.Warn("append split audit fail",
			zap.String("biz_type", bizType), zap.String("biz_id", bizID), zap.String("action", action), zap.Error(err))
	}
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
			expanded, err := s.expandAllStores(ctx, r.MerchantID, amount, i+1, time.Time{}, paidAt, nil)
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
// storeIDs 非空时仅在这些门店内按实收占比拆分（用于规则试算的门店选择）。
func (s *Service) expandAllStores(ctx context.Context, merchantID uint64, amount int64, level int, from, to time.Time, storeIDs []uint64) ([]executor.Allocation, error) {
	if s.revenueQuerier == nil {
		return nil, errs.New(errs.CodeInternalError, "store revenue querier not configured", 500)
	}
	revenues, err := s.revenueQuerier.SumPaidByStore(ctx, merchantID, from, to)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternalError, "query store revenue failed", 200, err)
	}
	// 过滤实收为 0 的门店，并按需仅保留指定门店（store_ids 过滤）
	valid := make([]repository.StoreRevenue, 0, len(revenues))
	storeSet := make(map[uint64]bool, len(storeIDs))
	for _, sid := range storeIDs {
		storeSet[sid] = true
	}
	var sum int64
	for _, rv := range revenues {
		if rv.Paid <= 0 {
			continue
		}
		if len(storeIDs) > 0 && !storeSet[rv.StoreID] {
			continue
		}
		valid = append(valid, rv)
		sum += rv.Paid
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
		allocations, err = s.buildAllocationsFiltered(ctx, matched, total, time.Time{}, req.StoreIDs)
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
		allocations, err = s.buildAllocationsPeriodFiltered(ctx, matched, total, start, end, req.StoreIDs)
		if err != nil {
			return nil, err
		}
	default:
		return nil, errs.New(errs.CodeInvalidParams, "amount or start/end required", 200)
	}

	items := allocationsToItems(allocations)
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

// buildAllocationsFiltered 单笔金额试算：同 buildAllocations，但 ALL_STORES 支持门店过滤。
func (s *Service) buildAllocationsFiltered(ctx context.Context, r *rule.Rule, total int64, paidAt time.Time, storeIDs []uint64) ([]executor.Allocation, error) {
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
			if remain := total - used; remain > 0 && remain != amount {
				amount = remain
			}
		}
		used += amount
		if a.ReceiverScope == "ALL_STORES" {
			expanded, err := s.expandAllStores(ctx, r.MerchantID, amount, i+1, time.Time{}, paidAt, storeIDs)
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
		return nil, errs.New(errs.CodeInvalidParams, "allocations exceed amount", 200)
	}
	return allocations, nil
}

// buildAllocationsPeriodFiltered 时间段试算：同 buildAllocationsPeriod，但 ALL_STORES 支持门店过滤。
func (s *Service) buildAllocationsPeriodFiltered(ctx context.Context, r *rule.Rule, total int64, start, end time.Time, storeIDs []uint64) ([]executor.Allocation, error) {
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
			if remain := total - used; remain > 0 && remain != amount {
				amount = remain
			}
		}
		used += amount
		if a.ReceiverScope == "ALL_STORES" {
			expanded, err := s.expandAllStores(ctx, r.MerchantID, amount, i+1, start, end, storeIDs)
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
		return nil, errs.New(errs.CodeInvalidParams, "allocations exceed amount", 200)
	}
	return allocations, nil
}

// RetryResult 重试结果。
type RetryResult struct {
	OrderNo  string `json:"order_no"`
	Success  int    `json:"success"` // 重试后成功接收方数
	Failed   int    `json:"failed"`  // 重试后失败接收方数
	Retried  int    `json:"retried"` // 本次实际重试的接收方数
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

	// 重建未成功接收方分配
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
		// 均已成功，无需重试
		return &RetryResult{OrderNo: orderNo, Success: len(rows), Failed: 0, Retried: 0}, nil
	}

	if s.account == nil {
		return nil, errs.New(errs.CodeInternalError, "account service not configured", 500)
	}
	merchantWallet, wErr := s.account.GetWalletByEntityType(ctx, merchantID, vo.EntityMerchant)
	if wErr != nil || merchantWallet == nil {
		return nil, errs.New(errs.CodeInternalError, "merchant wallet not found", 200)
	}

	if err := s.executor.Execute(ctx, &executor.ExecuteRequest{
		MerchantID:    merchantID,
		OrderNo:       orderNo,
		SourceWallet:  merchantWallet.ID,
		Allocations:   allocations,
		StoreID:       storeID,
		RuleID:        ruleID,
		Channel:       ch,
		IdempotencyKey: "split",
		TraceID:       "",
	}); err != nil {
		return nil, errs.Wrap(errs.CodeInternalError, "retry split execution failed", 200, err)
	}
	s.appendAudit(ctx, "EXECUTION", orderNo, "RETRY", merchantID, map[string]any{"retried": len(allocations)})
	prom.SplitRetryTotal.Inc()

	// 重试后统计
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

// ============ 差错中心 ============

// ExceptionItem 差错中心异常订单行。
type ExceptionItem struct {
	OrderNo       string     `json:"order_no"`
	RuleID        *uint64    `json:"rule_id"`
	TotalAmount   int64      `json:"total_amount"`
	ReceiverCount int        `json:"receiver_count"`
	SuccessCount  int        `json:"success_count"`
	Status        string     `json:"status"`
	AttemptCount  int        `json:"attempt_count"`
	NextRetryAt   *time.Time `json:"next_retry_at"`
	Degraded      int        `json:"degraded"`
	LastError     string     `json:"last_error"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// ExceptionPage 异常订单分页。
type ExceptionPage struct {
	Items []ExceptionItem `json:"items"`
	Total int64           `json:"total"`
	Page  int             `json:"page"`
	Size  int             `json:"size"`
}

// ListExceptions 差错中心异常订单聚合查询（FAILED/PARTIAL/SUSPENDED/DEAD/RESOLVED 或降级订单）。
func (s *Service) ListExceptions(ctx context.Context, merchantID uint64, status string, degraded *int, page, size int) (*ExceptionPage, error) {
	if s.orderStatusRepo == nil {
		return nil, errs.New(errs.CodeInternalError, "split order status repo not configured", 500)
	}
	rows, total, err := s.orderStatusRepo.ListExceptions(ctx, merchantID, status, degraded, (page-1)*size, size)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternalError, "query split exceptions failed", 200, err)
	}
	items := make([]ExceptionItem, 0, len(rows))
	for _, r := range rows {
		items = append(items, ExceptionItem{
			OrderNo:       r.OrderNo,
			RuleID:        r.RuleID,
			TotalAmount:   r.TotalAmount,
			ReceiverCount: r.ReceiverCount,
			SuccessCount:  r.SuccessCount,
			Status:        r.Status,
			AttemptCount:  r.AttemptCount,
			NextRetryAt:   r.NextRetryAt,
			Degraded:      r.Degraded,
			LastError:     r.LastError,
			CreatedAt:     r.CreatedAt,
			UpdatedAt:     r.UpdatedAt,
		})
	}
	return &ExceptionPage{Items: items, Total: total, Page: page, Size: size}, nil
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
	if st.Status != repository.OrderStatusDead {
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

// AuditItem 审计日志行。
type AuditItem struct {
	ID           uint64    `json:"id"`
	BizType      string    `json:"biz_type"`
	BizID        string    `json:"biz_id"`
	Action       string    `json:"action"`
	OperatorType string    `json:"operator_type"`
	OperatorID   uint64    `json:"operator_id"`
	Detail       string    `json:"detail"`
	CreatedAt    time.Time `json:"created_at"`
}

// AuditPage 审计日志分页。
type AuditPage struct {
	Items []AuditItem `json:"items"`
	Total int64       `json:"total"`
	Page  int         `json:"page"`
	Size  int         `json:"size"`
}

// ListAudits 商户端审计查询：biz_id 必填且强制校验归属（防越权查他商户审计）。
func (s *Service) ListAudits(ctx context.Context, merchantID uint64, bizType, bizID string, page, size int) (*AuditPage, error) {
	if s.auditRepo == nil {
		return nil, errs.New(errs.CodeInternalError, "split audit repo not configured", 500)
	}
	if bizID == "" {
		return nil, errs.New(errs.CodeInvalidParams, "biz_id required", 200)
	}
	owned, err := s.ownsBizID(ctx, merchantID, bizID)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternalError, "check biz ownership failed", 200, err)
	}
	if !owned {
		return nil, errs.New(errs.CodeInvalidParams, "biz not found", 200)
	}
	rows, total, err := s.auditRepo.List(ctx, bizType, bizID, "", (page-1)*size, size)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternalError, "query split audit failed", 200, err)
	}
	items := make([]AuditItem, 0, len(rows))
	for _, r := range rows {
		var detail string
		if r.Detail != nil {
			detail = *r.Detail
		}
		items = append(items, AuditItem{
			ID: r.ID, BizType: r.BizType, BizID: r.BizID, Action: r.Action,
			OperatorType: r.OperatorType, OperatorID: r.OperatorID,
			Detail: detail, CreatedAt: r.CreatedAt,
		})
	}
	return &AuditPage{Items: items, Total: total, Page: page, Size: size}, nil
}

// ownsBizID 校验 biz_id（订单号/批次号）归属当前商户。
func (s *Service) ownsBizID(ctx context.Context, merchantID uint64, bizID string) (bool, error) {
	// 订单级分账状态表自带 merchant_id，优先命中
	if s.orderStatusRepo != nil {
		st, err := s.orderStatusRepo.Get(ctx, bizID)
		if err != nil {
			return false, err
		}
		if st != nil {
			return st.MerchantID == merchantID, nil
		}
	}
	// 其次按交易订单 / 分账单批次归属判定
	var count int64
	err := s.diffRepo.DB().WithContext(ctx).Raw(
		`SELECT
			(SELECT COUNT(*) FROM t_order WHERE order_no = ? AND merchant_id = ? AND deleted_at IS NULL)
			+ (SELECT COUNT(*) FROM t_split_bill WHERE batch_no = ? AND merchant_id = ?)`,
		bizID, merchantID, bizID, merchantID,
	).Scan(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// DiffPage 对账差异分页。
type DiffPage struct {
	Items []repository.ReconcileDiffModel `json:"items"`
	Total int64                          `json:"total"`
	Page  int                            `json:"page"`
	Size  int                            `json:"size"`
}

// ListReconcileDiffs 商户端对账差异分页查询（强制商户隔离，支持类型/核销状态过滤）。
func (s *Service) ListReconcileDiffs(ctx context.Context, merchantID uint64, diffType string, resolved *bool, start, end time.Time, page, size int) (*DiffPage, error) {
	if s.diffRepo == nil {
		return nil, errs.New(errs.CodeInternalError, "reconcile diff repo not configured", 500)
	}
	rows, total, err := s.diffRepo.ListForMerchant(ctx, merchantID, diffType, resolved, start, end, (page-1)*size, size)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternalError, "query reconcile diffs failed", 200, err)
	}
	return &DiffPage{Items: rows, Total: total, Page: page, Size: size}, nil
}

// ResolveReconcileDiff 核销对账差异（乐观锁 + 审计留痕）。
func (s *Service) ResolveReconcileDiff(ctx context.Context, merchantID, id uint64) error {
	if s.diffRepo == nil {
		return errs.New(errs.CodeInternalError, "reconcile diff repo not configured", 500)
	}
	ok, err := s.diffRepo.Resolve(ctx, id, merchantID)
	if err != nil {
		return errs.Wrap(errs.CodeInternalError, "resolve reconcile diff failed", 200, err)
	}
	if !ok {
		return errs.New(errs.CodeInvalidParams, "差异不存在或已核销", 200)
	}
	s.appendAudit(ctx, repository.AuditBizTypeReconcileDiff, fmt.Sprintf("%d", id), repository.AuditActionResolve, merchantID, nil)
	return nil
}