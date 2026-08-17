// 包 recon 实现分账前置对账（Prechecker）。
//
// 双层对账（V2 评审要点）：
//   - Layer A 商户级总额：秒级阻断大偏差（LOWER/UPPER 不一致）
//   - Layer B 门店×日明细：定位错位门店
//
// 0 容差：任意一行 ≠ 一致即拒绝分账（用户要求）。
// LEFT JOIN 优化版：避免 NOT EXISTS 三层嵌套（V2 性能评审问题 🔴7）。
package recon

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/huipay/huipay-backend/infra/errs"
	"github.com/huipay/huipay-backend/infra/prom"
	"github.com/huipay/huipay-backend/internal/split/repository"
	"github.com/huipay/huipay-backend/internal/split/scope"
)

// Diff 对账差异行。
type Diff struct {
	StoreID    uint64 `json:"store_id"`
	BizDate    string `json:"biz_date"`
	OrderTotal int64  `json:"order_total"`
	StatsTotal int64  `json:"stats_total"`
	Diff       int64  `json:"diff"`
}

// StatsBackfiller 门店日报补跑接口（statsSvc 已实现 Backfill 与 HasMissing）。
type StatsBackfiller interface {
	HasMissing(ctx context.Context, merchantID uint64, start, end time.Time) (bool, error)
	Backfill(ctx context.Context, start, end time.Time) (int64, error)
}

// Prechecker 前置对账器。
type Prechecker struct {
	db        *gorm.DB
	statsSvc  StatsBackfiller
	diffRepo  *repository.ReconcileDiffRepo
	auditRepo *repository.SplitAuditRepo
	logger    *zap.Logger
}

// NewPrechecker 构造 Prechecker。
func NewPrechecker(db *gorm.DB, statsSvc StatsBackfiller, diffRepo *repository.ReconcileDiffRepo, auditRepo *repository.SplitAuditRepo, logger *zap.Logger) *Prechecker {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Prechecker{
		db: db, statsSvc: statsSvc, diffRepo: diffRepo, auditRepo: auditRepo, logger: logger,
	}
}

// CheckResult 对账结果。
type CheckResult struct {
	Pass      bool
	TotalDiff *int64      // 商户级差异（nil 表示未触发）
	Diffs     []Diff      // 门店×日差异（空表示无）
	DiffID    *uint64     // 写入的差异行 ID
}

// Check 执行双层对账。
func (p *Prechecker) Check(ctx context.Context, merchantID uint64, start, end time.Time) (*CheckResult, error) {
	// 1. 自动补跑：仅在日报缺失时（避免每次都跑）
	if p.statsSvc != nil {
		missing, mErr := p.statsSvc.HasMissing(ctx, merchantID, start, end)
		if mErr != nil {
			p.logger.Warn("prechecker has missing fail", zap.Error(mErr))
		} else if missing {
			if _, bErr := p.statsSvc.Backfill(ctx, start, end); bErr != nil {
				return nil, errs.New(errs.CodeStatsNotReady, "门店日报补跑失败，请重试", 200)
			}
		}
	}

	// 2. Layer A 商户级
	orderTotal, oErr := p.sumOrderTotal(ctx, merchantID, start, end)
	if oErr != nil {
		return nil, errs.Wrap(errs.CodeInternalError, "sum order total fail", 200, oErr)
	}
	statsTotal, sErr := p.sumStatsTotal(ctx, merchantID, start, end)
	if sErr != nil {
		return nil, errs.Wrap(errs.CodeInternalError, "sum stats total fail", 200, sErr)
	}
	if orderTotal != statsTotal {
		totalDiff := orderTotal - statsTotal
		diffID, wErr := p.diffRepo.WriteSplitPrecheck(ctx, merchantID, start, end, repository.DiffTypeSplitTotal, map[string]any{
			"order_total": orderTotal,
			"stats_total": statsTotal,
			"diff":        totalDiff,
		})
		if wErr != nil {
			p.logger.Warn("prechecker write total diff fail", zap.Error(wErr))
		}
		prom.SplitPrecheckDiffTotal.WithLabelValues("TOTAL").Inc()
		_ = p.auditRepo.WriteAction(ctx, repository.AuditBizTypeDailySplit,
			formatBizDate(start), repository.AuditActionReconcileFailed,
			repository.AuditOperatorSystem, 0,
			map[string]any{"level": "TOTAL", "order_total": orderTotal, "stats_total": statsTotal, "diff": totalDiff})
		return &CheckResult{Pass: false, TotalDiff: &totalDiff, DiffID: &diffID}, errs.New(
			errs.CodeReconcileFailedTotal,
			fmt.Sprintf("商户级总额不平：订单合计=%d分 门店日报合计=%d分 差=%d分", orderTotal, statsTotal, totalDiff),
			200,
		)
	}

	// 3. Layer B 门店×日
	orderRows, oErr := p.sumOrderByStoreAndDate(ctx, merchantID, start, end)
	if oErr != nil {
		return nil, errs.Wrap(errs.CodeInternalError, "sum order by store fail", 200, oErr)
	}
	statsRows, sErr := p.fetchStatsByStoreAndDate(ctx, merchantID, start, end)
	if sErr != nil {
		return nil, errs.Wrap(errs.CodeInternalError, "fetch stats rows fail", 200, sErr)
	}
	diffs := compareRows(orderRows, statsRows)
	if len(diffs) > 0 {
		diffID, wErr := p.diffRepo.WriteSplitPrecheck(ctx, merchantID, start, end, repository.DiffTypeSplitDetail, diffs)
		if wErr != nil {
			p.logger.Warn("prechecker write detail diff fail", zap.Error(wErr))
		}
		prom.SplitPrecheckDiffTotal.WithLabelValues("DETAIL").Inc()
		_ = p.auditRepo.WriteAction(ctx, repository.AuditBizTypeDailySplit,
			formatBizDate(start), repository.AuditActionReconcileFailed,
			repository.AuditOperatorSystem, 0,
			map[string]any{"level": "DETAIL", "diff_count": len(diffs), "diffs": diffs})
		return &CheckResult{Pass: false, Diffs: diffs, DiffID: &diffID}, errs.New(
			errs.CodeReconcileFailedDetail,
			fmt.Sprintf("门店×日不平：%d 条差异", len(diffs)),
			200,
		)
	}

	// 4. 通过
	prom.SplitPrecheckDiffTotal.WithLabelValues("PASS").Inc()
	_ = p.auditRepo.WriteAction(ctx, repository.AuditBizTypeDailySplit,
		formatBizDate(start), repository.AuditActionReconcilePassed,
		repository.AuditOperatorSystem, 0,
		map[string]any{"order_total": orderTotal, "stats_total": statsTotal, "merchant_id": merchantID})
	return &CheckResult{Pass: true}, nil
}

func (p *Prechecker) sumOrderTotal(ctx context.Context, merchantID uint64, from, to time.Time) (int64, error) {
	var total int64
	row := p.db.WithContext(ctx).Raw(scope.LayerAQuery, merchantID, from, to).Row()
	if err := row.Scan(&total); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, nil
		}
		return 0, err
	}
	return total, nil
}

func (p *Prechecker) sumStatsTotal(ctx context.Context, merchantID uint64, from, to time.Time) (int64, error) {
	var total int64
	row := p.db.WithContext(ctx).Raw(scope.StatsSumQuery, merchantID, from, to).Row()
	if err := row.Scan(&total); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, nil
		}
		return 0, err
	}
	return total, nil
}

func (p *Prechecker) sumOrderByStoreAndDate(ctx context.Context, merchantID uint64, from, to time.Time) (map[string]int64, error) {
	type row struct {
		BizDate    time.Time `gorm:"column:biz_date"`
		StoreID    uint64    `gorm:"column:store_id"`
		OrderTotal int64     `gorm:"column:order_total"`
	}
	var rows []row
	if err := p.db.WithContext(ctx).Raw(scope.LayerBQuery, merchantID, from, to).Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make(map[string]int64, len(rows))
	for _, r := range rows {
		key := fmt.Sprintf("%s|%d", r.BizDate.Format("2006-01-02"), r.StoreID)
		out[key] = r.OrderTotal
	}
	return out, nil
}

func (p *Prechecker) fetchStatsByStoreAndDate(ctx context.Context, merchantID uint64, from, to time.Time) (map[string]int64, error) {
	type row struct {
		BizDate    time.Time `gorm:"column:biz_date"`
		StoreID    uint64    `gorm:"column:store_id"`
		StatsTotal int64     `gorm:"column:stats_total"`
	}
	var rows []row
	if err := p.db.WithContext(ctx).Raw(scope.StatsRowsQuery, merchantID, from, to).Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make(map[string]int64, len(rows))
	for _, r := range rows {
		key := fmt.Sprintf("%s|%d", r.BizDate.Format("2006-01-02"), r.StoreID)
		out[key] = r.StatsTotal
	}
	return out, nil
}

// compareRows 比对两侧聚合结果，返回差异行（0 容差）。
func compareRows(order, stats map[string]int64) []Diff {
	keys := make(map[string]struct{}, len(order)+len(stats))
	for k := range order {
		keys[k] = struct{}{}
	}
	for k := range stats {
		keys[k] = struct{}{}
	}
	var diffs []Diff
	for k := range keys {
		o := order[k]
		s := stats[k]
		if o == s {
			continue
		}
		// 解析 key "YYYY-MM-DD|store_id"
		var d Diff
		biz, sidStr, ok := strings.Cut(k, "|")
		if ok {
			d.BizDate = biz
			if v, perr := strconv.ParseUint(sidStr, 10, 64); perr == nil {
				d.StoreID = v
			}
		}
		d.OrderTotal = o
		d.StatsTotal = s
		d.Diff = o - s
		diffs = append(diffs, d)
	}
	return diffs
}

func formatBizDate(d time.Time) string {
	return d.Format("2006-01-02")
}