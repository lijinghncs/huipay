// Package alloc 提供分账分配方案的纯函数计算——不依赖任何外部存储或服务。
package alloc

import (
	"fmt"
	"sort"
	"time"

	"github.com/huipay/huipay-backend/infra/errs"
	"github.com/huipay/huipay-backend/internal/domain/vo"
	"github.com/huipay/huipay-backend/internal/split/executor"
	"github.com/huipay/huipay-backend/internal/split/rule"
)

// StoreRevenue 门店实收数据（纯函数输入，不依赖 repository 层）。
type StoreRevenue struct {
	StoreID uint64
	Paid    int64
}

// Input 分配计算输入。
type Input struct {
	Rule          *rule.Rule
	Total         int64
	StoreRevenues []StoreRevenue // 仅 ALL_STORES 展开时需要；nil 时跳过展开
	FilterStoreIDs []uint64      // 可选：限定参与分摊的门店（空=全部门店）
}

// Compute 计算分配方案，纯函数。
// 1) 按规则分配项计算金额（比例/固定额）
// 2) 末笔取整补齐（仅比例合计 100% 时）
// 3) ALL_STORES 按门店实收占比展开
// 4) 校验分配总额不超过输入总额
func Compute(in Input) ([]executor.Allocation, error) {
	if in.Rule == nil {
		return nil, errs.New(errs.CodeInternalError, "split rule is nil", 500)
	}
	if len(in.Rule.Allocations) == 0 {
		return nil, errs.New(errs.CodeSplitRuleNotMatch, "split rule has no allocations", 200)
	}

	allocations := make([]executor.Allocation, 0, len(in.Rule.Allocations))
	var ratioSum int64
	for _, a := range in.Rule.Allocations {
		ratioSum += a.RatioBps
	}
	var used int64
	for i, a := range in.Rule.Allocations {
		amount := a.FixedAmount
		if a.RatioBps > 0 {
			amount = in.Total * a.RatioBps / 10000
		}
		if amount <= 0 {
			return nil, errs.New(errs.CodeInvalidParams, "invalid allocation amount", 200)
		}
		// 末笔补齐仅用于比例合计 100% 时的取整误差；部分分账（合计 < 100%）不补齐
		if i == len(in.Rule.Allocations)-1 && ratioSum == 10000 {
			remain := in.Total - used
			if remain > 0 && remain != amount {
				amount = remain
			}
		}
		used += amount

		if a.ReceiverScope == "ALL_STORES" {
			expanded, err := expandAllStores(in.StoreRevenues, amount, i+1, in.FilterStoreIDs)
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
	if used > in.Total {
		return nil, errs.New(errs.CodeInvalidParams, "allocations exceed total amount", 200)
	}
	return allocations, nil
}

// expandAllStores 将某分配项金额按门店实收占比拆分为逐店子分配。
// storeIDs 非空时仅在这些门店内按实收占比拆分。
func expandAllStores(revenues []StoreRevenue, amount int64, level int, storeIDs []uint64) ([]executor.Allocation, error) {
	// 过滤实收为 0 的门店，并按需仅保留指定门店
	storeSet := make(map[uint64]bool, len(storeIDs))
	for _, sid := range storeIDs {
		storeSet[sid] = true
	}
	valid := make([]StoreRevenue, 0, len(revenues))
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

// ParsePaidAt 解析订单支付时间（RFC3339），解析失败返回零值。
func ParsePaidAt(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

// BillItem 账单明细行（与 repository.SplitBillItem 等价，但不上溯到 repository 包）。
type BillItem struct {
	ReceiverEntityID uint64 `json:"receiver_entity_id"`
	ReceiverType     string `json:"receiver_type"`
	ReceiverName     string `json:"receiver_name"`
	Amount           int64  `json:"amount"`
}

// AllocationsToItems 将执行单元转换为账单明细。
func AllocationsToItems(list []executor.Allocation) []BillItem {
	items := make([]BillItem, 0, len(list))
	for _, a := range list {
		items = append(items, BillItem{
			ReceiverEntityID: a.EntityID,
			ReceiverType:     string(a.EntityType),
			Amount:           a.Amount,
		})
	}
	return items
}

// ItemsToAllocations 将账单明细转换为执行单元。
func ItemsToAllocations(items []BillItem) []executor.Allocation {
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

// CollectBizDates 收集 [start, end) 区间内每个自然日。
func CollectBizDates(start, end time.Time) []time.Time {
	out := []time.Time{}
	for d := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.UTC); d.Before(end); d = d.AddDate(0, 0, 1) {
		out = append(out, d)
	}
	return out
}

// TruncateMsg 截断错误信息至 1000 字符。
func TruncateMsg(s string) string {
	const max = 1000
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

// FormatBatchNo 生成确定性分账批次号。
func FormatBatchNo(ruleID uint64, start, end time.Time) string {
	return fmt.Sprintf("SP%d-%d-%d", ruleID, start.Unix(), end.Unix())
}