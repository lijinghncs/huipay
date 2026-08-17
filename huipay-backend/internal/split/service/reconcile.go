package service

import (
	"context"
	"time"

	"github.com/huipay/huipay-backend/infra/errs"
	"github.com/huipay/huipay-backend/internal/split/repository"
)

// ExceptionPage 异常分页结果。
type ExceptionPage struct {
	Items []repository.SplitOrderStatusModel `json:"items"`
	Total int64                              `json:"total"`
}

// DiffPage 对账差异分页结果。
type DiffPage struct {
	Items []repository.ReconcileDiffModel `json:"items"`
	Total int64                           `json:"total"`
}

// AuditItem 审计记录项。
type AuditItem struct {
	ID         uint64      `json:"id"`
	BizType    string      `json:"biz_type"`
	BizID      string      `json:"biz_id"`
	Action     string      `json:"action"`
	OperatorID uint64      `json:"operator_id"`
	Detail     interface{} `json:"detail,omitempty"`
	CreatedAt  string      `json:"created_at"`
}

// AuditPage 审计记录分页结果。
type AuditPage struct {
	Items []AuditItem `json:"items"`
	Total int64       `json:"total"`
}

// ListExceptions 查询异常分账列表。
func (s *Service) ListExceptions(ctx context.Context, merchantID uint64, status string, degraded *int, page, size int) (*ExceptionPage, error) {
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 200 {
		size = 20
	}
	items, total, err := s.orderStatusRepo.ListExceptions(ctx, merchantID, status, degraded, (page-1)*size, size)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternalError, "query exceptions failed", 200, err)
	}
	return &ExceptionPage{Items: items, Total: total}, nil
}

// ListReconcileDiffs 查询对账差异列表。
func (s *Service) ListReconcileDiffs(ctx context.Context, merchantID uint64, diffType string, resolved *bool, start, end time.Time, page, size int) (*DiffPage, error) {
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

// ResolveReconcileDiff 标记对账差异为已处理。
func (s *Service) ResolveReconcileDiff(ctx context.Context, merchantID uint64, diffID uint64) error {
	ok, err := s.diffRepo.Resolve(ctx, diffID, merchantID)
	if err != nil {
		return errs.Wrap(errs.CodeInternalError, "resolve reconcile diff failed", 200, err)
	}
	if !ok {
		return errs.New(errs.CodeInvalidParams, "diff not found or not owned by merchant", 200)
	}
	s.appendAudit(ctx, repository.AuditBizTypeReconcileDiff, "diff", "RESOLVE", merchantID, map[string]any{"diff_id": diffID})
	return nil
}

// ListAudits 查询审计记录。
func (s *Service) ListAudits(ctx context.Context, merchantID uint64, bizType, bizID string, page, size int) (*AuditPage, error) {
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 200 {
		size = 20
	}
	items, total, err := s.auditRepo.List(ctx, bizType, bizID, "", (page-1)*size, size)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternalError, "query audits failed", 200, err)
	}
	out := make([]AuditItem, 0, len(items))
	for _, a := range items {
		out = append(out, AuditItem{
			ID:         a.ID,
			BizType:    a.BizType,
			BizID:      a.BizID,
			Action:     a.Action,
			OperatorID: a.OperatorID,
			Detail:     a.Detail,
			CreatedAt:  a.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}
	return &AuditPage{Items: out, Total: total}, nil
}