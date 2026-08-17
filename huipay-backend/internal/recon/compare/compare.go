// Package compare 提供对账比对的纯函数核心：总额比对、按 key 明细比对、渠道账单逐笔匹配。
// 零外部依赖（仅依赖 recon/domain），可脱离数据库独立单测。
package compare

import (
	"github.com/huipay/huipay-backend/internal/recon/domain"
)

// Totals 比对两侧总额，返回差异金额（0 表示一致）。
func Totals(local, remote int64) int64 { return local - remote }

// RowDiff 按 key 比对的单行差异。
type RowDiff[K comparable] struct {
	Key          K
	LocalAmount  int64
	RemoteAmount int64
}

// Rows 按 key 逐一比对两侧聚合，返回差异行（零容差）。
// 本地有而对端无、对端有而本地无、两侧金额不等均计为差异。
func Rows[K comparable](local, remote map[K]int64) []RowDiff[K] {
	var out []RowDiff[K]
	for k, v := range local {
		if remote[k] != v {
			out = append(out, RowDiff[K]{Key: k, LocalAmount: v, RemoteAmount: remote[k]})
		}
	}
	for k, v := range remote {
		if _, ok := local[k]; !ok {
			out = append(out, RowDiff[K]{Key: k, LocalAmount: 0, RemoteAmount: v})
		}
	}
	return out
}

// BillReport 渠道对账比对结果（LONG / SHORT / MISMATCH 三分类）。
type BillReport struct {
	Long     []domain.Diff // 本地有、渠道无
	Short    []domain.Diff // 渠道有、本地无
	Mismatch []domain.Diff // 金额不一致
}

// MatchBills 将本地订单与渠道账单逐笔匹配：先按渠道交易号，再按商户订单号。
func MatchBills(local []domain.LocalOrder, bill []domain.BillEntry) BillReport {
	var rep BillReport

	byTxn := make(map[string]domain.BillEntry, len(bill))
	byOut := make(map[string]domain.BillEntry, len(bill))
	for _, e := range bill {
		if e.TransactionID != "" {
			byTxn[e.TransactionID] = e
		}
		if e.OutTradeNo != "" {
			byOut[e.OutTradeNo] = e
		}
	}

	matched := make(map[string]struct{}, len(bill))
	for _, o := range local {
		var entry domain.BillEntry
		var ok bool
		if o.ChannelTradeNo != "" {
			entry, ok = byTxn[o.ChannelTradeNo]
		}
		if !ok {
			entry, ok = byOut[o.OrderNo]
		}
		if !ok {
			rep.Long = append(rep.Long, domain.Diff{
				OrderNo:       o.OrderNo,
				TransactionID: o.ChannelTradeNo,
				LocalAmount:   domain.Int64(o.PaidAmount),
				Reason:        domain.ReasonLocalOnly,
			})
			continue
		}
		if entry.TransactionID != "" {
			matched[entry.TransactionID] = struct{}{}
		}
		if entry.OutTradeNo != "" {
			matched[entry.OutTradeNo] = struct{}{}
		}
		if o.PaidAmount != entry.OrderAmount {
			rep.Mismatch = append(rep.Mismatch, domain.Diff{
				OrderNo:       o.OrderNo,
				TransactionID: entry.TransactionID,
				LocalAmount:   domain.Int64(o.PaidAmount),
				RemoteAmount:  domain.Int64(entry.OrderAmount),
				Reason:        domain.ReasonMismatch,
			})
		}
	}

	for _, e := range bill {
		if _, ok := matched[e.TransactionID]; ok {
			continue
		}
		if _, ok := matched[e.OutTradeNo]; ok {
			continue
		}
		rep.Short = append(rep.Short, domain.Diff{
			OrderNo:       e.OutTradeNo,
			TransactionID: e.TransactionID,
			RemoteAmount:  domain.Int64(e.OrderAmount),
			Reason:        domain.ReasonRemoteOnly,
		})
	}
	return rep
}
