package service

import (
	"context"
	"encoding/json"

	"github.com/huipay/huipay-backend/infra/errs"
	"github.com/huipay/huipay-backend/internal/split/repository"
	"github.com/huipay/huipay-backend/internal/split/rule"
)

// RuleDTO 分账规则 DTO（HTTP 响应）。
type RuleDTO struct {
	ID          uint64          `json:"id"`
	MerchantID  uint64          `json:"merchant_id"`
	RuleCode    string          `json:"rule_code"`
	RuleName    string          `json:"rule_name"`
	Priority    int             `json:"priority"`
	Status      int             `json:"status"`
	Allocations []rule.Allocation `json:"allocations"`
}

// CreateRuleRequest 创建规则请求。
type CreateRuleRequest struct {
	RuleCode    string          `json:"rule_code" binding:"required"`
	RuleName    string          `json:"rule_name" binding:"required"`
	Priority    int             `json:"priority"`
	Allocations []rule.Allocation `json:"allocations" binding:"required,min=1"`
	Description string          `json:"description,omitempty"`
}

// UpdateRuleRequest 更新规则请求。
type UpdateRuleRequest struct {
	RuleName    string          `json:"rule_name"`
	Priority    int             `json:"priority"`
	Allocations []rule.Allocation `json:"allocations"`
	Status      int             `json:"status"`
	Description string          `json:"description,omitempty"`
}

// ruleToDTO 转换 SplitRuleModel 为 RuleDTO。
func ruleToDTO(m *repository.SplitRuleModel) *RuleDTO {
	dto := &RuleDTO{
		ID:         m.ID,
		MerchantID: m.MerchantID,
		RuleCode:   m.RuleCode,
		RuleName:   m.RuleName,
		Priority:   m.Priority,
		Status:     m.Status,
	}
	if allocations, err := rule.ParseAllocations(m.Allocations); err == nil {
		dto.Allocations = allocations
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
		out = append(out, RuleDTO{
			ID:          r.ID,
			MerchantID:  r.MerchantID,
			RuleCode:    r.RuleCode,
			RuleName:    r.RuleName,
			Priority:    r.Priority,
			Status:      r.Status,
			Allocations: r.Allocations,
		})
	}
	return out, nil
}

// CreateRule 创建分账规则。
func (s *Service) CreateRule(ctx context.Context, merchantID uint64, req *CreateRuleRequest) (*RuleDTO, error) {
	if s.ruleRepo == nil {
		return nil, errs.New(errs.CodeInternalError, "split rule repo not configured", 500)
	}
	existing, err := s.ruleRepo.GetByCodeAndMerchant(ctx, req.RuleCode, merchantID)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternalError, "check split rule failed", 200, err)
	}
	if existing != nil {
		return nil, errs.New(errs.CodeInvalidParams, "rule_code already exists", 200)
	}

	allocJSON, err := json.Marshal(req.Allocations)
	if err != nil {
		return nil, errs.New(errs.CodeInvalidParams, "invalid allocations", 200)
	}

	m := &repository.SplitRuleModel{
		RuleCode:    req.RuleCode,
		MerchantID:  merchantID,
		RuleName:    req.RuleName,
		Priority:    req.Priority,
		Allocations: string(allocJSON),
		Status:      1,
	}
	if err := s.ruleRepo.Create(ctx, m); err != nil {
		return nil, errs.Wrap(errs.CodeInternalError, "create split rule failed", 200, err)
	}
	return ruleToDTO(m), nil
}

// UpdateRule 更新分账规则。
func (s *Service) UpdateRule(ctx context.Context, id uint64, merchantID uint64, req *UpdateRuleRequest) (*RuleDTO, error) {
	if s.ruleRepo == nil {
		return nil, errs.New(errs.CodeInternalError, "split rule repo not configured", 500)
	}
	existing, err := s.ruleRepo.GetByID(ctx, id, merchantID)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternalError, "query split rule failed", 200, err)
	}
	if existing == nil {
		return nil, errs.New(errs.CodeInvalidParams, "split rule not found", 200)
	}

	fields := make(map[string]any)
	if req.RuleName != "" {
		fields["rule_name"] = req.RuleName
	}
	if req.Priority > 0 {
		fields["priority"] = req.Priority
	}
	if len(req.Allocations) > 0 {
		allocJSON, err := json.Marshal(req.Allocations)
		if err != nil {
			return nil, errs.New(errs.CodeInvalidParams, "invalid allocations", 200)
		}
		fields["allocations"] = string(allocJSON)
	}
	if req.Status > 0 {
		fields["status"] = req.Status
	}
	if req.Description != "" {
		fields["description"] = req.Description
	}

	if len(fields) > 0 {
		if err := s.ruleRepo.Update(ctx, id, merchantID, fields); err != nil {
			return nil, errs.Wrap(errs.CodeInternalError, "update split rule failed", 200, err)
		}
	}

	updated, err := s.ruleRepo.GetByID(ctx, id, merchantID)
	if err != nil || updated == nil {
		return nil, errs.New(errs.CodeInternalError, "query updated rule failed", 200)
	}
	return ruleToDTO(updated), nil
}

// SetRuleStatus 启用/停用分账规则。
func (s *Service) SetRuleStatus(ctx context.Context, id uint64, merchantID uint64, status int) error {
	if s.ruleRepo == nil {
		return errs.New(errs.CodeInternalError, "split rule repo not configured", 500)
	}
	existing, err := s.ruleRepo.GetByID(ctx, id, merchantID)
	if err != nil {
		return errs.Wrap(errs.CodeInternalError, "query split rule failed", 200, err)
	}
	if existing == nil {
		return errs.New(errs.CodeInvalidParams, "split rule not found", 200)
	}
	return s.ruleRepo.UpdateStatus(ctx, id, merchantID, status)
}

// DeleteRule 删除分账规则（硬删除）。
func (s *Service) DeleteRule(ctx context.Context, id uint64, merchantID uint64) error {
	if s.ruleRepo == nil {
		return errs.New(errs.CodeInternalError, "split rule repo not configured", 500)
	}
	existing, err := s.ruleRepo.GetByID(ctx, id, merchantID)
	if err != nil {
		return errs.Wrap(errs.CodeInternalError, "query split rule failed", 200, err)
	}
	if existing == nil {
		return errs.New(errs.CodeInvalidParams, "split rule not found", 200)
	}
	return s.ruleRepo.Delete(ctx, id, merchantID)
}