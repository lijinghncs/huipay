package service

import (
	"context"
	"time"

	"github.com/huipay/huipay-backend/infra/errs"
	"github.com/huipay/huipay-backend/internal/split/repository"
	"github.com/huipay/huipay-backend/internal/split/rule"
)

// RuleDTO 分账规则 DTO（HTTP 响应）。
type RuleDTO struct {
	ID            uint64    `json:"id"`
	MerchantID    uint64    `json:"merchant_id"`
	RuleCode      string    `json:"rule_code"`
	RuleName      string    `json:"rule_name"`
	RuleType      string    `json:"rule_type"`
	Priority      int       `json:"priority"`
	Status        string    `json:"status"`
	Allocations   []rule.AllocationItem `json:"allocations"`
	EffectiveAt   string    `json:"effective_at,omitempty"`
	ExpireAt      string    `json:"expire_at,omitempty"`
	Description   string    `json:"description,omitempty"`
	CreatedAt     string    `json:"created_at"`
	UpdatedAt     string    `json:"updated_at"`
}

// CreateRuleRequest 创建规则请求。
type CreateRuleRequest struct {
	RuleCode    string                `json:"rule_code" binding:"required"`
	RuleName    string                `json:"rule_name" binding:"required"`
	RuleType    string                `json:"rule_type" binding:"required"`
	Priority    int                   `json:"priority"`
	Allocations []rule.AllocationItem `json:"allocations" binding:"required,min=1"`
	EffectiveAt string                `json:"effective_at,omitempty"`
	ExpireAt    string                `json:"expire_at,omitempty"`
	Description string                `json:"description,omitempty"`
}

// UpdateRuleRequest 更新规则请求。
type UpdateRuleRequest struct {
	RuleName    string                `json:"rule_name"`
	RuleType    string                `json:"rule_type"`
	Priority    int                   `json:"priority"`
	Allocations []rule.AllocationItem `json:"allocations"`
	Status      string                `json:"status"`
	EffectiveAt string                `json:"effective_at,omitempty"`
	ExpireAt    string                `json:"expire_at,omitempty"`
	Description string                `json:"description,omitempty"`
}

// ruleToDTO 内部转换。
func (s *Service) ruleToDTO(r *rule.Rule) *RuleDTO {
	dto := &RuleDTO{
		ID:          r.ID,
		MerchantID:  r.MerchantID,
		RuleCode:    r.RuleCode,
		RuleName:    r.RuleName,
		RuleType:    r.RuleType,
		Priority:    r.Priority,
		Status:      r.Status,
		Allocations: r.Allocations,
		CreatedAt:   r.CreatedAt.Format(time.RFC3339),
		UpdatedAt:   r.UpdatedAt.Format(time.RFC3339),
	}
	if !r.EffectiveAt.IsZero() {
		dto.EffectiveAt = r.EffectiveAt.Format(time.RFC3339)
	}
	if !r.ExpireAt.IsZero() {
		dto.ExpireAt = r.ExpireAt.Format(time.RFC3339)
	}
	return dto
}

// ListRules 查询商户分账规则列表。
func (s *Service) ListRules(ctx context.Context, merchantID uint64) ([]RuleDTO, error) {
	if s.ruleRepo == nil {
		return nil, errs.New(errs.CodeInternalError, "split rule repo not configured", 500)
	}
	rules, err := s.ruleRepo.ListByMerchant(ctx, merchantID)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternalError, "query split rules failed", 200, err)
	}
	out := make([]RuleDTO, 0, len(rules))
	for _, r := range rules {
		out = append(out, *s.ruleToDTO(r))
	}
	return out, nil
}

// CreateRule 创建分账规则。
func (s *Service) CreateRule(ctx context.Context, merchantID uint64, req *CreateRuleRequest) (*RuleDTO, error) {
	if s.ruleRepo == nil {
		return nil, errs.New(errs.CodeInternalError, "split rule repo not configured", 500)
	}
	// 检查规则编码是否已存在
	existing, err := s.ruleRepo.GetByCodeAndMerchant(ctx, req.RuleCode, merchantID)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternalError, "check split rule failed", 200, err)
	}
	if existing != nil {
		return nil, errs.New(errs.CodeInvalidParams, "rule_code already exists", 200)
	}

	r := &rule.Rule{
		MerchantID:    merchantID,
		RuleCode:      req.RuleCode,
		RuleName:      req.RuleName,
		RuleType:      req.RuleType,
		Priority:      req.Priority,
		Status:        "ACTIVE",
		Allocations:   req.Allocations,
		Description:   req.Description,
	}
	if req.EffectiveAt != "" {
		if t, err := time.Parse(time.RFC3339, req.EffectiveAt); err == nil {
			r.EffectiveAt = t
		}
	}
	if req.ExpireAt != "" {
		if t, err := time.Parse(time.RFC3339, req.ExpireAt); err == nil {
			r.ExpireAt = t
		}
	}

	if err := s.ruleRepo.Create(ctx, r); err != nil {
		return nil, errs.Wrap(errs.CodeInternalError, "create split rule failed", 200, err)
	}
	return s.ruleToDTO(r), nil
}

// UpdateRule 更新分账规则（全量覆盖）。
func (s *Service) UpdateRule(ctx context.Context, merchantID uint64, ruleID uint64, req *UpdateRuleRequest) (*RuleDTO, error) {
	if s.ruleRepo == nil {
		return nil, errs.New(errs.CodeInternalError, "split rule repo not configured", 500)
	}
	r, err := s.ruleRepo.GetByID(ctx, ruleID)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternalError, "query split rule failed", 200, err)
	}
	if r == nil || r.MerchantID != merchantID {
		return nil, errs.New(errs.CodeInvalidParams, "split rule not found", 200)
	}

	if req.RuleName != "" {
		r.RuleName = req.RuleName
	}
	if req.RuleType != "" {
		r.RuleType = req.RuleType
	}
	if req.Priority > 0 {
		r.Priority = req.Priority
	}
	if len(req.Allocations) > 0 {
		r.Allocations = req.Allocations
	}
	if req.Status != "" {
		r.Status = req.Status
	}
	if req.EffectiveAt != "" {
		if t, err := time.Parse(time.RFC3339, req.EffectiveAt); err == nil {
			r.EffectiveAt = t
		}
	}
	if req.ExpireAt != "" {
		if t, err := time.Parse(time.RFC3339, req.ExpireAt); err == nil {
			r.ExpireAt = t
		}
	}
	if req.Description != "" {
		r.Description = req.Description
	}

	if err := s.ruleRepo.Update(ctx, r); err != nil {
		return nil, errs.Wrap(errs.CodeInternalError, "update split rule failed", 200, err)
	}
	return s.ruleToDTO(r), nil
}

// SetRuleStatus 启用/停用分账规则。
func (s *Service) SetRuleStatus(ctx context.Context, merchantID uint64, ruleID uint64, status string) error {
	if s.ruleRepo == nil {
		return errs.New(errs.CodeInternalError, "split rule repo not configured", 500)
	}
	r, err := s.ruleRepo.GetByID(ctx, ruleID)
	if err != nil {
		return errs.Wrap(errs.CodeInternalError, "query split rule failed", 200, err)
	}
	if r == nil || r.MerchantID != merchantID {
		return errs.New(errs.CodeInvalidParams, "split rule not found", 200)
	}
	if status != "ACTIVE" && status != "INACTIVE" {
		return errs.New(errs.CodeInvalidParams, "status must be ACTIVE or INACTIVE", 200)
	}
	return s.ruleRepo.UpdateStatus(ctx, ruleID, status)
}

// DeleteRule 删除分账规则（软删除）。
func (s *Service) DeleteRule(ctx context.Context, merchantID uint64, ruleID uint64) error {
	if s.ruleRepo == nil {
		return errs.New(errs.CodeInternalError, "split rule repo not configured", 500)
	}
	r, err := s.ruleRepo.GetByID(ctx, ruleID)
	if err != nil {
		return errs.Wrap(errs.CodeInternalError, "query split rule failed", 200, err)
	}
	if r == nil || r.MerchantID != merchantID {
		return errs.New(errs.CodeInvalidParams, "split rule not found", 200)
	}
	return s.ruleRepo.Delete(ctx, ruleID)
}