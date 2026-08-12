// 包 repository 提供分账规则(t_split_rule)数据访问。
package repository

import (
	"context"
	"time"

	"gorm.io/gorm"

	"github.com/huipay/huipay-backend/internal/split/rule"
)

// SplitRuleModel 分账规则表 GORM 模型（t_split_rule）。
type SplitRuleModel struct {
	ID           uint64     `gorm:"column:id;primaryKey;autoIncrement"`
	RuleCode     string     `gorm:"column:rule_code;size:32;uniqueIndex:uk_rule_code;not null"`
	MerchantID   uint64     `gorm:"column:merchant_id;not null"`
	RuleName     string     `gorm:"column:rule_name;size:128;not null"`
	Priority     int        `gorm:"column:priority;not null;default:0"`
	Conditions   string     `gorm:"column:conditions;type:json;not null"`
	Allocations  string     `gorm:"column:allocations;type:json;not null"`
	TriggerType  string     `gorm:"column:trigger_type;size:32;not null;default:PAID"`
	Status       int        `gorm:"column:status;not null;default:1"`
	EffectiveFrom *time.Time `gorm:"column:effective_from"`
	EffectiveTo  *time.Time `gorm:"column:effective_to"`
}

// TableName 表名。
func (SplitRuleModel) TableName() string { return "t_split_rule" }

// SplitRuleRepo 分账规则仓储。
type SplitRuleRepo struct{ db *gorm.DB }

// NewSplitRuleRepo 构造 SplitRuleRepo。
func NewSplitRuleRepo(db *gorm.DB) *SplitRuleRepo { return &SplitRuleRepo{db: db} }

// ListByMerchant 查询商户启用中的分账规则，解析为规则引擎模型。
func (r *SplitRuleRepo) ListByMerchant(ctx context.Context, merchantID uint64) ([]rule.Rule, error) {
	var rows []SplitRuleModel
	if err := r.db.WithContext(ctx).
		Where("merchant_id = ? AND status = 1", merchantID).
		Order("priority DESC").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	list := make([]rule.Rule, 0, len(rows))
	for i := range rows {
		conditions, err := rule.ParseConditions(rows[i].Conditions)
		if err != nil {
			return nil, err
		}
		allocations, err := rule.ParseAllocations(rows[i].Allocations)
		if err != nil {
			return nil, err
		}
		list = append(list, rule.Rule{
			ID:          rows[i].ID,
			RuleCode:    rows[i].RuleCode,
			MerchantID:  rows[i].MerchantID,
			Priority:    rows[i].Priority,
			Conditions:  conditions,
			Allocations: allocations,
			Status:      rows[i].Status,
		})
	}
	return list, nil
}

// GetByCodeAndMerchant 按规则编码 + 商户查询单条规则。
func (r *SplitRuleRepo) GetByCodeAndMerchant(ctx context.Context, ruleCode string, merchantID uint64) (*rule.Rule, error) {
	var row SplitRuleModel
	if err := r.db.WithContext(ctx).
		Where("rule_code = ? AND merchant_id = ? AND status = 1", ruleCode, merchantID).
		First(&row).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	conditions, err := rule.ParseConditions(row.Conditions)
	if err != nil {
		return nil, err
	}
	allocations, err := rule.ParseAllocations(row.Allocations)
	if err != nil {
		return nil, err
	}
	return &rule.Rule{
		ID:          row.ID,
		RuleCode:    row.RuleCode,
		MerchantID:  row.MerchantID,
		Priority:    row.Priority,
		Conditions:  conditions,
		Allocations: allocations,
		Status:      row.Status,
	}, nil
}