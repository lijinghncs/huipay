// Package channel 实现渠道对账（T+1）：本地已支付订单与渠道账单逐笔核对。
package channel

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/huipay/huipay-backend/internal/recon/compare"
	"github.com/huipay/huipay-backend/internal/recon/domain"
	"github.com/huipay/huipay-backend/internal/recon/ports"
)

// TaskName 调度任务名（对账中心运行日志与手动触发均使用）。
const TaskName = "reconcile_daily"

// detailText 差异明细落库文案（与既有渠道对账落库文本保持一致）。
var detailText = map[string]string{
	domain.ReasonLocalOnly:  "local only",
	domain.ReasonRemoteOnly: "channel only",
	domain.ReasonMismatch:   "amount mismatch",
}

// Job 渠道对账任务。
type Job struct {
	orders ports.OrderSideFetcher
	bills  ports.ChannelBillFetcher
	diffs  ports.DiffStore
	logger *zap.Logger
}

func NewJob(orders ports.OrderSideFetcher, bills ports.ChannelBillFetcher, diffs ports.DiffStore, logger *zap.Logger) *Job {
	return &Job{orders: orders, bills: bills, diffs: diffs, logger: logger}
}

func (j *Job) Name() string { return TaskName }

// Run 对账指定业务日期（T-1），返回差异行数。
func (j *Job) Run(ctx context.Context, bizDate time.Time) (int64, error) {
	dateStr := bizDate.Format("2006-01-02")

	entries, err := j.bills.FetchBill(ctx, dateStr)
	if err != nil {
		return 0, fmt.Errorf("download channel bill: %w", err)
	}

	start := time.Date(bizDate.Year(), bizDate.Month(), bizDate.Day(), 0, 0, 0, 0, time.Local)
	end := start.AddDate(0, 0, 1)
	orders, err := j.orders.ListPaidForChannel(ctx, start, end)
	if err != nil {
		return 0, fmt.Errorf("query local orders: %w", err)
	}

	rep := compare.MatchBills(orders, entries)

	// SHORT 单商户归属：经 t_order 关联回填，关联不上保持 NULL。
	var shortOrderNos []string
	for _, d := range rep.Short {
		if d.OrderNo != "" {
			shortOrderNos = append(shortOrderNos, d.OrderNo)
		}
	}
	shortMerchants, err := j.orders.MerchantsByOrderNos(ctx, shortOrderNos)
	if err != nil {
		return 0, fmt.Errorf("backfill short-order merchant: %w", err)
	}
	merchantByOrder := make(map[string]uint64, len(orders))
	for _, o := range orders {
		merchantByOrder[o.OrderNo] = o.MerchantID
	}

	// 按商户分组落库（幂等清理粒度为 diff_type + 商户 + 业务日期）。
	type groupKey struct {
		diffType   string
		merchantID uint64
		hasMerchant bool
	}
	groups := make(map[groupKey][]domain.Diff)
	add := func(diffType string, merchantID uint64, hasMerchant bool, d domain.Diff) {
		d.Detail = detailText[d.Reason]
		k := groupKey{diffType: diffType, merchantID: merchantID, hasMerchant: hasMerchant}
		groups[k] = append(groups[k], d)
	}
	for _, d := range rep.Long {
		add(domain.DiffTypeLong, merchantByOrder[d.OrderNo], true, d)
	}
	for _, d := range rep.Mismatch {
		add(domain.DiffTypeMismatch, merchantByOrder[d.OrderNo], true, d)
	}
	for _, d := range rep.Short {
		if mid, ok := shortMerchants[d.OrderNo]; ok {
			add(domain.DiffTypeShort, mid, true, d)
		} else {
			add(domain.DiffTypeShort, 0, false, d)
		}
	}

	var total int64
	for k, rows := range groups {
		var mid *uint64
		if k.hasMerchant {
			v := k.merchantID
			mid = &v
		}
		n, err := j.diffs.WriteOrderDiffs(ctx, mid, start, k.diffType, rows)
		if err != nil {
			return total, fmt.Errorf("write channel diffs (type %s): %w", k.diffType, err)
		}
		total += int64(n)
	}

	j.logger.Info("channel reconcile done",
		zap.String("biz_date", dateStr),
		zap.Int("local_orders", len(orders)),
		zap.Int("bill_entries", len(entries)),
		zap.Int("long", len(rep.Long)),
		zap.Int("short", len(rep.Short)),
		zap.Int("mismatch", len(rep.Mismatch)))
	return total, nil
}
