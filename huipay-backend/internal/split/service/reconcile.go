package service

import (
	"context"
	"time"

	"github.com/huipay/huipay-backend/infra/errs"
	"github.com/huipay/huipay-backend/internal/split/repository"
)

// DiffPage 对账差异分页结果。
type DiffPage struct {
	Items []repository.ReconcileDiff `json:"items"`
	Total int64                      `json:"total"`
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
	items, total, err := s.diffRepo.ListByMerchant(ctx, merchantID, diffType, resolved, start, end, (page-1)*size, size)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternalError, "query reconcile diffs failed", 200, err)
	}
	return &DiffPage{Items: items, Total: total}, nil
}

// ResolveReconcileDiff 标记对账差异为已处理（手动确认）。
func (s *Service) ResolveReconcileDiff(ctx context.Context, merchantID uint64, diffID uint64) error {
	if s.diffRepo == nil {
		return errs.New(errs.CodeInternalError, "diff repo not configured", 500)
	}
	diff, err := s.diffRepo.GetByID(ctx, diffID)
	if err != nil {
		return errs.Wrap(errs.CodeInternalError, "query reconcile diff failed", 200, err)
	}
	if diff == nil || diff.MerchantID != merchantID {
		return errs.New(errs.CodeInvalidParams, "reconcile diff not found", 200)
	}
	if diff.Status != "OPEN" {
		return errs.New(errs.CodeInvalidParams, "diff already resolved", 200)
	}
	if err := s.diffRepo.UpdateStatus(ctx, diffID, "RESOLVED", ""); err != nil {
		return errs.Wrap(errs.CodeInternalError, "update diff status failed", 200, err)
	}
	s.appendAudit(ctx, "RECONCILE", "diff", "RESOLVE", merchantID, map[string]any{"diff_id": diffID})
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
	Items []ExceptionItem `json:"items"`
	Total int64           `json:"total"`
}

// ListExceptions 查询异常分账记录（FAILED / SUSPENDED / DEAD），支持按状态过滤。
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
	statuses := []string{
		repository.OrderStatusFailed,
		repository.OrderStatusSuspended,
		repository.OrderStatusDead,
	}
	rows, total, err := s.orderStatusRepo.ListByStatuses(ctx, merchantID, statuses, (page-1)*size, size)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternalError, "query split exceptions failed", 200, err)
	}
	items := make([]ExceptionItem, 0, len(rows))
	for _, r := range rows {
		items = append(items, ExceptionItem{
			OrderNo:    r.OrderNo,
			MerchantID: r.MerchantID,
			Status:     r.Status,
			Amount:     r.Amount,
			FailedAt:   r.UpdatedAt.Format(time.RFC3339),
		})
	}
	return &ExceptionPage{Items: items, Total: total}, nil
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
	Items []AuditItem `json:"items"`
	Total int64       `json:"total"`
}

// ListAudits 查询审计日志，支持按 biz_id 过滤。
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
	rows, total, err := s.auditRepo.ListByMerchant(ctx, merchantID, bizType, bizID, (page-1)*size, size)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternalError, "query audit logs failed", 200, err)
	}
	items := make([]AuditItem, 0, len(rows))
	for _, r := range rows {
		items = append(items, AuditItem{
			BizType:   r.BizType,
			BizID:     r.BizID,
			Action:    r.Action,
			Details:   r.Details,
			CreatedAt: r.CreatedAt.Format(time.RFC3339),
		})
	}
	return &AuditPage{Items: items, Total: total}, nil
}