// 包 service 提供管理后台分账管理：每日执行、审计、对账差异、重算/重置。
package service

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	reconrepo "github.com/huipay/huipay-backend/internal/recon/repository"
	"github.com/huipay/huipay-backend/internal/split/repository"
	statsrepo "github.com/huipay/huipay-backend/internal/stats/repository"
	statsservice "github.com/huipay/huipay-backend/internal/stats/service"
)

// SplitManageService 分账管理后台服务（每日执行/审计/差异/重算重置）。
type SplitManageService struct {
	db        *gorm.DB
	dailyRepo *repository.DailyExecutionRepo
	auditRepo *repository.SplitAuditRepo
	diffRepo  *reconrepo.DiffStore
	statsSvc  *statsservice.Service
	logger    *zap.Logger
}

// NewSplitManageService 构造服务。
func NewSplitManageService(
	db *gorm.DB,
	dailyRepo *repository.DailyExecutionRepo,
	auditRepo *repository.SplitAuditRepo,
	diffRepo *reconrepo.DiffStore,
	statsSvc *statsservice.Service,
	logger *zap.Logger,
) *SplitManageService {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &SplitManageService{
		db: db, dailyRepo: dailyRepo, auditRepo: auditRepo, diffRepo: diffRepo,
		statsSvc: statsSvc, logger: logger,
	}
}

// DailyExecPage 每日执行分页结果。
type DailyExecPage struct {
	Items []repository.DailyExecutionModel `json:"items"`
	Total int64                            `json:"total"`
}

// DailyExecFilter 每日执行筛选。
type DailyExecFilter struct {
	MerchantID uint64
	Start      time.Time
	End        time.Time
	Status     string
	Page       int
	PageSize   int
}

// ListDailyExecutions 分页查询每日执行记录。
func (s *SplitManageService) ListDailyExecutions(ctx context.Context, f DailyExecFilter) (*DailyExecPage, error) {
	if f.Page < 1 {
		f.Page = 1
	}
	if f.PageSize <= 0 || f.PageSize > 200 {
		f.PageSize = 20
	}
	rows, total, err := s.dailyRepo.ListByMerchantDateRange(ctx, f.MerchantID, f.Start, f.End, f.Status, (f.Page-1)*f.PageSize, f.PageSize)
	if err != nil {
		return nil, err
	}
	return &DailyExecPage{Items: rows, Total: total}, nil
}

// GetDailyExecution 单次执行详情。
func (s *SplitManageService) GetDailyExecution(ctx context.Context, id uint64) (*repository.DailyExecutionModel, error) {
	return s.dailyRepo.GetByID(ctx, id)
}

// AuditPage 审计分页结果。
type AuditPage struct {
	Items []repository.SplitAuditModel `json:"items"`
	Total int64                        `json:"total"`
}

// AuditFilter 审计筛选。
type AuditFilter struct {
	BizType  string
	BizID    string
	Action   string
	Page     int
	PageSize int
}

// ListAudits 分页查询审计记录。
func (s *SplitManageService) ListAudits(ctx context.Context, f AuditFilter) (*AuditPage, error) {
	if f.Page < 1 {
		f.Page = 1
	}
	if f.PageSize <= 0 || f.PageSize > 200 {
		f.PageSize = 20
	}
	rows, total, err := s.auditRepo.List(ctx, f.BizType, f.BizID, f.Action, (f.Page-1)*f.PageSize, f.PageSize)
	if err != nil {
		return nil, err
	}
	return &AuditPage{Items: rows, Total: total}, nil
}

// DiffPage 对账差异分页结果。
type DiffPage struct {
	Items []reconrepo.DiffModel `json:"items"`
	Total int64                 `json:"total"`
}

// DiffFilter 对账差异筛选。
type DiffFilter struct {
	MerchantID uint64
	DiffType   string
	Start      time.Time
	End        time.Time
	Page       int
	PageSize   int
}

// ListDiffs 分页查询对账差异。
func (s *SplitManageService) ListDiffs(ctx context.Context, f DiffFilter) (*DiffPage, error) {
	if f.Page < 1 {
		f.Page = 1
	}
	if f.PageSize <= 0 || f.PageSize > 200 {
		f.PageSize = 20
	}
	rows, total, err := s.diffRepo.ListByMerchantAndType(ctx, f.MerchantID, f.DiffType, nil, f.Start, f.End, (f.Page-1)*f.PageSize, f.PageSize)
	if err != nil {
		return nil, err
	}
	return &DiffPage{Items: rows, Total: total}, nil
}

// RecomputeStoreStats 主动触发单个 (merchant, biz_date) 的 split_status 回算 + 审计。
func (s *SplitManageService) RecomputeStoreStats(ctx context.Context, merchantID uint64, bizDate time.Time, operatorID uint64) (int, error) {
	n, err := s.statsSvc.RecomputeSplitStatus(ctx, merchantID, bizDate)
	if err != nil {
		return 0, err
	}
	_ = s.auditRepo.WriteAction(ctx, repository.AuditBizTypeDailySplit,
		bizDate.Format("2006-01-02"), "RECOMPUTE",
		repository.AuditOperatorAdmin, operatorID,
		map[string]any{"updated_stores": n, "merchant_id": merchantID})
	return n, nil
}

// ResetStoreSplitStatus 重置单门店×日分账状态 + 审计。
func (s *SplitManageService) ResetStoreSplitStatus(ctx context.Context, merchantID, storeID uint64, bizDate time.Time, operatorID uint64) (bool, error) {
	ok, err := s.statsSvc.ResetSplitStatus(ctx, merchantID, storeID, bizDate)
	if err != nil {
		return false, err
	}
	if ok {
		key := bizDate.Format("2006-01-02")
		_ = s.auditRepo.WriteAction(ctx, repository.AuditBizTypeDailySplit,
			key, repository.AuditActionReset,
			repository.AuditOperatorAdmin, operatorID,
			map[string]any{"merchant_id": merchantID, "store_id": storeID})
	}
	return ok, nil
}

// ResolveExecution 管理端人工核销死单/异常分账订单：置 RESOLVED 终态（差错闭环）+ 审计。
// note 为线下处理说明。返回 false 表示订单不存在或当前状态不可核销。
func (s *SplitManageService) ResolveExecution(ctx context.Context, orderNo, note string, operatorID uint64) (bool, error) {
	osr := repository.NewSplitOrderStatusRepo(s.db)
	ok, err := osr.MarkResolved(ctx, orderNo)
	if err != nil {
		return false, err
	}
	if ok {
		_ = s.auditRepo.WriteAction(ctx, repository.AuditBizTypeSplitExec,
			orderNo, repository.AuditActionResolve,
			repository.AuditOperatorAdmin, operatorID,
			map[string]any{"note": note})
	}
	return ok, nil
}

// ReopenExecution 管理端死单复位重开：DEAD → FAILED 并清零重试计数，交由补偿调度重入 + 审计。
// 返回 false 表示订单不存在或当前状态不可复位。
func (s *SplitManageService) ReopenExecution(ctx context.Context, orderNo string, operatorID uint64) (bool, error) {
	osr := repository.NewSplitOrderStatusRepo(s.db)
	ok, err := osr.Reopen(ctx, orderNo)
	if err != nil {
		return false, err
	}
	if ok {
		_ = s.auditRepo.WriteAction(ctx, repository.AuditBizTypeSplitExec,
			orderNo, repository.AuditActionReopen,
			repository.AuditOperatorAdmin, operatorID,
			map[string]any{})
	}
	return ok, nil
}

// ResolveReconcileDiff 管理端核销对账差异（跨商户）+ 审计。
func (s *SplitManageService) ResolveReconcileDiff(ctx context.Context, id, operatorID uint64) (bool, error) {
	ok, err := s.diffRepo.ResolveByID(ctx, id)
	if err != nil {
		return false, err
	}
	if ok {
		_ = s.auditRepo.WriteAction(ctx, repository.AuditBizTypeReconcileDiff,
			fmt.Sprintf("%d", id), repository.AuditActionResolve,
			repository.AuditOperatorAdmin, operatorID,
			map[string]any{})
	}
	return ok, nil
}

// ExceptionItem 差错中心异常订单行（管理端跨商户）。
type ExceptionItem struct {
	OrderNo       string     `json:"order_no"`
	MerchantID    uint64     `json:"merchant_id"`
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

// ExceptionPage 异常订单分页结果。
type ExceptionPage struct {
	Items []ExceptionItem `json:"items"`
	Total int64           `json:"total"`
	Page  int             `json:"page"`
	Size  int             `json:"size"`
}

// ListExceptions 管理端跨商户异常订单聚合查询（FAILED/PARTIAL/SUSPENDED/DEAD/RESOLVED 或降级订单）。
func (s *SplitManageService) ListExceptions(ctx context.Context, status string, degraded *int, page, size int) (*ExceptionPage, error) {
	rows, total, err := repository.NewSplitOrderStatusRepo(s.db).ListAllExceptions(ctx, status, degraded, (page-1)*size, size)
	if err != nil {
		return nil, err
	}
	items := make([]ExceptionItem, 0, len(rows))
	for _, r := range rows {
		items = append(items, ExceptionItem{
			OrderNo:       r.OrderNo,
			MerchantID:    r.MerchantID,
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

// ensure statsrepo import retained for future extensions
var _ = statsrepo.StoreDailyStatsModel{}