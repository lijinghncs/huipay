// Package service 编排分账业务。
//
// 按 UseCase 拆分为 5 个文件：
//   service.go     — Service 结构体、NewService、共享基础设施
//   ordersplit.go  — 单笔订单分账（Execute / Get / Preview / Retry / ListExecutions）
//   periodbill.go  — 时段分账 + 审批流（ExecuteByPeriod / GenerateBill / ApproveBill / RejectBill）
//   reconcile.go   — 差错中心 + 对账差异（ListExceptions / ListReconcileDiffs / ResolveReconcileDiff / ListAudits）
//   rules.go       — 规则 CRUD（ListRules / CreateRule / UpdateRule / SetRuleStatus / DeleteRule）
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/huipay/huipay-backend/infra/errs"
	"github.com/huipay/huipay-backend/internal/account/service"
	"github.com/huipay/huipay-backend/internal/domain/vo"
	"github.com/huipay/huipay-backend/internal/split/alloc"
	"github.com/huipay/huipay-backend/internal/split/executor"
	"github.com/huipay/huipay-backend/internal/split/recon"
	"github.com/huipay/huipay-backend/internal/split/repository"
	"github.com/huipay/huipay-backend/internal/split/rule"
	statsservice "github.com/huipay/huipay-backend/internal/stats/service"
)

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

// appendAudit 追加分账审计记录（D2；审计仓储未配置时静默忽略，不阻断业务流程）。
func (s *Service) appendAudit(ctx context.Context, bizType, bizID, action string, operatorID uint64, detail any) {
	if s.auditRepo == nil {
		return
	}
	var detailJSON *string
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

// readExecutionFacts 反查 t_split_execution 真实状态。
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
		msg = alloc.TruncateMsg(cause.Error())
	}
	runID := formatRunIDFail(merchantID, batchNo)
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

// ownsBizID 校验 biz_id（订单号/批次号）归属当前商户。
func (s *Service) ownsBizID(ctx context.Context, merchantID uint64, bizID string) (bool, error) {
	if s.orderStatusRepo != nil {
		st, err := s.orderStatusRepo.Get(ctx, bizID)
		if err != nil {
			return false, err
		}
		if st != nil {
			return st.MerchantID == merchantID, nil
		}
	}
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

// formatRunIDFail 生成失败运行 ID。
func formatRunIDFail(merchantID uint64, batchNo string) string {
	return fmt.Sprintf("SP_RUN-FAIL-%d-%s-%d", merchantID, batchNo, time.Now().UnixNano())
}