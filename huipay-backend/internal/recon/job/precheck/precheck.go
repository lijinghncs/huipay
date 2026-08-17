// Package precheck 实现前置对账（Layer A 总额 + Layer B 明细），
// 在分账执行前同步触发，不通过则阻断执行。
package precheck

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/huipay/huipay-backend/infra/errs"
	"github.com/huipay/huipay-backend/internal/recon/compare"
	"github.com/huipay/huipay-backend/internal/recon/domain"
	"github.com/huipay/huipay-backend/internal/recon/ports"
)

// 指标 label。
const (
	LabelTotal  = "TOTAL"
	LabelDetail = "DETAIL"
	LabelPass   = "PASS"
)

// TaskName 前置对账运行日志任务名（对账中心可查询）。
const TaskName = "split_precheck"

// Prechecker 前置对账检查器。
type Prechecker struct {
	orders    ports.OrderSideFetcher
	stats     ports.StatsSideFetcher
	diffs     ports.DiffStore
	audit     ports.AuditRecorder
	observer  ports.Observer
	runLogger ports.RunLogger // 可选，nil 时不补记运行日志
	logger    *zap.Logger
}

func NewPrechecker(orders ports.OrderSideFetcher, stats ports.StatsSideFetcher, diffs ports.DiffStore, audit ports.AuditRecorder, observer ports.Observer, runLogger ports.RunLogger, logger *zap.Logger) *Prechecker {
	return &Prechecker{orders: orders, stats: stats, diffs: diffs, audit: audit, observer: observer, runLogger: runLogger, logger: logger}
}

// totalDetail Layer A 差异明细（保持既有落库 JSON 结构）。
type totalDetail struct {
	OrderTotal int64 `json:"order_total"`
	StatsTotal int64 `json:"stats_total"`
	Diff       int64 `json:"diff"`
}

// detailRow Layer B 差异明细行（保持既有落库 JSON 结构）。
type detailRow struct {
	StoreID    uint64 `json:"store_id"`
	BizDate    string `json:"biz_date"`
	OrderTotal int64  `json:"order_total"`
	StatsTotal int64  `json:"stats_total"`
	Diff       int64  `json:"diff"`
}

// Check 执行前置对账。
// 前置动作：日报缺失时自动补跑；Layer A 总额比对；Layer B 门店×日期明细比对。
func (p *Prechecker) Check(ctx context.Context, merchantID uint64, start, end time.Time) (*domain.CheckResult, error) {
	start = time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.Local)
	end = time.Date(end.Year(), end.Month(), end.Day(), 0, 0, 0, 0, time.Local)

	// 0. 日报缺失时自动补跑
	missing, err := p.stats.HasMissing(ctx, merchantID, start, end)
	if err != nil {
		return nil, err
	}
	if missing {
		if _, err := p.stats.Backfill(ctx, start, end); err != nil {
			return nil, fmt.Errorf("backfill daily stats: %w", err)
		}
	}

	// Layer A：商户级总额比对
	orderTotal, err := p.orders.SumForSplit(ctx, merchantID, start, end)
	if err != nil {
		return nil, fmt.Errorf("query order total: %w", err)
	}
	statsTotal, err := p.stats.Sum(ctx, merchantID, start, end)
	if err != nil {
		return nil, fmt.Errorf("query stats total: %w", err)
	}
	if diff := compare.Totals(orderTotal, statsTotal); diff != 0 {
		p.observer.ObservePrecheck(LabelTotal)
		diffID := p.writeTotalDiff(ctx, merchantID, start, end, orderTotal, statsTotal, diff)
		p.writeAudit(ctx, start, false, orderTotal, statsTotal, diff, nil)
		p.logFailedRun(ctx, start, 1, fmt.Sprintf("total mismatch: order=%d stats=%d diff=%d", orderTotal, statsTotal, diff))
		return &domain.CheckResult{Pass: false, TotalDiff: &diff, DiffID: diffID},
			errs.New(errs.CodeReconcileFailedTotal,
				fmt.Sprintf("对账失败：总额不一致（order=%d stats=%d diff=%d）", orderTotal, statsTotal, diff), 200)
	}

	// Layer B：门店 × 日期 明细比对
	orderRows, err := p.orders.SumByStoreAndDate(ctx, merchantID, start, end)
	if err != nil {
		return nil, fmt.Errorf("query order rows: %w", err)
	}
	statsRows, err := p.stats.Rows(ctx, merchantID, start, end)
	if err != nil {
		return nil, fmt.Errorf("query stats rows: %w", err)
	}
	rowDiffs := compare.Rows(orderRows, statsRows)
	if len(rowDiffs) == 0 {
		p.observer.ObservePrecheck(LabelPass)
		p.writeAudit(ctx, start, true, orderTotal, statsTotal, 0, nil)
		return &domain.CheckResult{Pass: true}, nil
	}

	p.observer.ObservePrecheck(LabelDetail)
	diffs := toDiffs(rowDiffs)
	diffID := p.writeDetailDiff(ctx, merchantID, start, end, diffs)
	p.writeAudit(ctx, start, false, orderTotal, statsTotal, 0, diffs)
	p.logFailedRun(ctx, start, int64(len(diffs)), fmt.Sprintf("detail mismatch: %d rows", len(diffs)))
	return &domain.CheckResult{Pass: false, Diffs: diffs, DiffID: diffID},
		errs.New(errs.CodeReconcileFailedDetail, fmt.Sprintf("对账失败：门店×日期不一致（%d 条）", len(diffs)), 200)
}

// toDiffs 将按 key 比对结果转为领域差异行（key 格式 "biz_date|store_id"）。
func toDiffs(rowDiffs []compare.RowDiff[string]) []domain.Diff {
	diffs := make([]domain.Diff, 0, len(rowDiffs))
	for _, rd := range rowDiffs {
		bizDate, storeID := rd.Key, uint64(0)
		if parts := strings.SplitN(rd.Key, "|", 2); len(parts) == 2 {
			bizDate = parts[0]
			if v, err := strconv.ParseUint(parts[1], 10, 64); err == nil {
				storeID = v
			}
		}
		diffs = append(diffs, domain.Diff{
			StoreID:      storeID,
			BizDate:      bizDate,
			LocalAmount:  domain.Int64(rd.LocalAmount),
			RemoteAmount: domain.Int64(rd.RemoteAmount),
		})
	}
	return diffs
}

// writeTotalDiff 落库 Layer A 差异；失败仅告警不阻断。
func (p *Prechecker) writeTotalDiff(ctx context.Context, merchantID uint64, start, end time.Time, orderTotal, statsTotal, diff int64) *uint64 {
	detail, _ := json.Marshal(totalDetail{OrderTotal: orderTotal, StatsTotal: statsTotal, Diff: diff})
	diffID, err := p.diffs.WritePrecheck(ctx, merchantID, start, end, domain.DiffTypeSplitTotal, string(detail))
	if err != nil {
		p.logger.Warn("precheck: write total diff failed", zap.Uint64("merchant_id", merchantID), zap.Error(err))
		return nil
	}
	return &diffID
}

// writeDetailDiff 落库 Layer B 差异；失败仅告警不阻断。
func (p *Prechecker) writeDetailDiff(ctx context.Context, merchantID uint64, start, end time.Time, diffs []domain.Diff) *uint64 {
	rows := make([]detailRow, 0, len(diffs))
	for _, d := range diffs {
		rows = append(rows, detailRow{
			StoreID:    d.StoreID,
			BizDate:    d.BizDate,
			OrderTotal: d.Local(),
			StatsTotal: d.Remote(),
			Diff:       d.Amount(),
		})
	}
	detail, _ := json.Marshal(rows)
	diffID, err := p.diffs.WritePrecheck(ctx, merchantID, start, end, domain.DiffTypeSplitDetail, string(detail))
	if err != nil {
		p.logger.Warn("precheck: write detail diff failed", zap.Uint64("merchant_id", merchantID), zap.Error(err))
		return nil
	}
	return &diffID
}

// writeAudit 记录对账审计（通过/失败均记录）。
func (p *Prechecker) writeAudit(ctx context.Context, start time.Time, passed bool, orderTotal, statsTotal, diff int64, diffs []domain.Diff) {
	action := domain.AuditActionReconcilePassed
	if !passed {
		action = domain.AuditActionReconcileFailed
	}
	detail := map[string]any{
		"order_total": orderTotal,
		"stats_total": statsTotal,
		"diff":        diff,
	}
	if len(diffs) > 0 {
		detail["diff_count"] = len(diffs)
		detail["diffs"] = diffs
	}
	p.audit.Record(ctx, domain.AuditBizTypeDailySplit, formatBizDate(start), action, domain.AuditOperatorSystem, 0, detail)
}

// logFailedRun 对账失败时补记对账中心运行日志（状态 FAILED）。
func (p *Prechecker) logFailedRun(ctx context.Context, bizDate time.Time, rows int64, summary string) {
	if p.runLogger == nil {
		return
	}
	p.runLogger.Log(ctx, TaskName, bizDate, rows, fmt.Errorf("%s", summary))
}

func formatBizDate(t time.Time) string { return t.Format("2006-01-02") }
