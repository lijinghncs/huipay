// 包 rule 实现分账规则 DSL 解析与匹配（骨架）。
// P2 阶段完整实现：模板 + 触发器 + 上下文（按渠道/门店/活动/时段/客户标签）。
package rule

import (
	"encoding/json"
	"sort"
)

// Condition 规则触发条件（JSON 序列化）。
type Condition struct {
	Channel   string   `json:"channel,omitempty"`
	StoreIDs  []uint64 `json:"store_ids,omitempty"`
	StartAt   string   `json:"start_at,omitempty"`
	EndAt     string   `json:"end_at,omitempty"`
	Tag       string   `json:"tag,omitempty"`
}

// Allocation 分配方案。
type Allocation struct {
	ReceiverEntityID uint64 `json:"receiver_entity_id"`
	ReceiverType     string `json:"receiver_type"`
	RatioBps         int64  `json:"ratio_bps"`     // 万分比（10000 = 100%）
	FixedAmount      int64  `json:"fixed_amount"`  // 固定金额（分），与比例互斥
}

// Rule 分账规则。
type Rule struct {
	ID          uint64
	RuleCode    string
	MerchantID  uint64
	Priority    int
	Conditions  Condition
	Allocations []Allocation
	Status      int
}

// ParseConditions 解析条件 JSON。
func ParseConditions(raw string) (Condition, error) {
	var c Condition
	if raw == "" {
		return c, nil
	}
	err := json.Unmarshal([]byte(raw), &c)
	return c, err
}

// ParseAllocations 解析分配方案 JSON。
func ParseAllocations(raw string) ([]Allocation, error) {
	var list []Allocation
	if raw == "" {
		return list, nil
	}
	err := json.Unmarshal([]byte(raw), &list)
	return list, err
}

// MatchContext 匹配上下文。
type MatchContext struct {
	MerchantID uint64
	Channel    string
	StoreID    uint64
	NowAt   string
}

// Match 判断是否匹配。
func (r *Rule) Match(ctx MatchContext) bool {
	if r.MerchantID != 0 && r.MerchantID != ctx.MerchantID {
		return false
	}
	if r.Conditions.Channel != "" && r.Conditions.Channel != ctx.Channel {
		return false
	}
	if len(r.Conditions.StoreIDs) > 0 {
		ok := false
		for _, sid := range r.Conditions.StoreIDs {
			if sid == ctx.StoreID {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	return true
}

// Engine 规则引擎。
type Engine struct{}

// NewEngine 构造 Engine。
func NewEngine() *Engine { return &Engine{} }

// Resolve 在给定规则集合中按 priority 倒序挑选首个匹配。
func (e *Engine) Resolve(rules []Rule, ctx MatchContext) *Rule {
	matched := make([]Rule, 0)
	for _, r := range rules {
		if r.Match(ctx) {
			matched = append(matched, r)
		}
	}
	if len(matched) == 0 {
		return nil
	}
	sort.SliceStable(matched, func(i, j int) bool { return matched[i].Priority > matched[j].Priority })
	return &matched[0]
}