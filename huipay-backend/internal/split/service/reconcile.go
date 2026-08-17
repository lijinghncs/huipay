package service

import (
	"context"
	"time"

	"github.com/huipay/huipay-backend/infra/errs"
	"github.com/huipay/huipay-backend/internal/split/repository"
)

// DiffPage 对账差异分页结果。
type DiffPage struct {
	Items []repository.ReconcileDiffModel `json:"items"`
	Total int64                           `json:"total"`
}

// ListReconcileDiffs 分页查询对账差异（差错中心），按类型/时间/核销状态过滤。
func (s *Service) ListReconcileDiffs(ctx context.Context, merchantID uint64, diffType string, resolved *bool, start, end time.Time, page, size int) (*DiffPage, error) {
	if s.diffRepo == nil {
		return nil, errs.New(errs.CodeInternalError, "diff repo not configured", 500)
	}
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 200 {
		size = 20
	}
	items, total, err := s.diffRepo.ListForMerchant(ctx, merchantID, diffType, resolved, start, end, (page-1)*size, size)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternalError, "query reconcile diffs failed", 200, err)
	}
	return &DiffPage{Items: items, Total: total}, nil
}

// ResolveReconcileDiff 标记对账差异为已处理（手动核销）。
func (s *Service) ResolveReconcileDiff(ctx context.Context, merchantID uint64, diffID uint64) error {
	if s.diffRepo == nil {
		return errs.New(errs.CodeInternalError, "diff repo not configured", 500)
	}
	ok, err := s.diffRepo.Resolve(ctx, diffID, merchantID)
	if err != nil {
		return errs.Wrap(errs.CodeInternalError, "resolve reconcile diff failed", 200, err)
	}
	if !ok {
		return errs.New(errs.CodeInvalidParams, "diff not found or already resolved", 200)
	}
	s.appendAudit(ctx, repository.AuditBizTypeReconcileDiff, "diff", "RESOLVE", merchantID, map[string]any{"diff_id": diffID})
	return nil
}

// ExceptionItem 异常分账条目。
type ExceptionItem struct {
	OrderNo      string `json:"order_no"`
	MerchantID   uint64 `json:"merchant_id"`
	Status       string `json:"status"`
	Amount       int64  `json:"amount"`
	RuleCode     string `json:"rule_code,omitempty"`
	FailedAt     string `json:"failed_at,omitempty"`
	ErrorMessage string `json:"error_message,omitempty"`
}

// ExceptionPage 异常分页结果。
type ExceptionPage struct {
	Items []repository.SplitOrderStatusModel `json:"items"`
	Total int64                              `json:"total"`
}

// ListExceptions 查询异常分账记录（FAILED / PARTIAL / SUSPENDED / DEAD / RESOLVED），支持按状态过滤。
func (s *Service) ListExceptions(ctx context.Context, merchantID uint64, status string, degraded *int, page, size int) (*ExceptionPage, error) {
	if s.orderStatusRepo == nil {
		return nil, errs.New(errs.CodeInternalError, "order status repo not configured", 500)
	}
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 200 {
		size = 20
	}
	rows, total, err := s.orderStatusRepo.ListExceptions(ctx, merchantID, status, degraded, (page-1)*size, size)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternalError, "query split exceptions failed", 200, err)
	}
	return &ExceptionPage{Items: rows, Total: total}, nil
}

// AuditItem 审计日志条目。
type AuditItem struct {
	BizType   string `json:"biz_type"`
	BizID     string `json:"biz_id"`
	Action    string `json:"action"`
	Details   string `json:"details,omitempty"`
	CreatedAt string `json:"created_at"`
}

// AuditPage 审计日志分页结果。
type AuditPage struct {
	Items []repository.SplitAuditModel `json:"items"`
	Total int64                        `json:"total"`
}

// ListAudits 查询审计日志，支持按 biz_type/biz_id 过滤。
func (s *Service) ListAudits(ctx context.Context, merchantID uint64, bizType, bizID string, page, size int) (*AuditPage, error) {
	if s.auditRepo == nil {
		return nil, errs.New(errs.CodeInternalError, "audit repo not configured", 500)
	}
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 200 {
		size = 20
	}
	rows, total, err := s.auditRepo.List(ctx, bizType, bizID, "", (page-1)*size, size)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternalError, "query audit logs failed", 200, err)
	}
	return &AuditPage{Items: rows, Total: total}, nil
}