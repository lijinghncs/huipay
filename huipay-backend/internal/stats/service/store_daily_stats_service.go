// 包 service 编排门店订单日报生成与报表查询。
package service

import (
	"context"
	"encoding/json"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	statsrepo "github.com/huipay/huipay-backend/internal/stats/repository"
)

// Service 门店日报服务。
type Service struct {
	repo   *statsrepo.StoreDailyStatsRepo
	db     *gorm.DB
	logger *zap.Logger
}

// NewService 构造 Service。
func NewService(repo *statsrepo.StoreDailyStatsRepo, db *gorm.DB, logger *zap.Logger) *Service {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Service{repo: repo, db: db, logger: logger}
}

// StoreDailyStatsModel 复用仓储模型。
type StoreDailyStatsModel = statsrepo.StoreDailyStatsModel

// GenerateDailyStats 聚合 T 日订单生成门店日报（单条 SQL，幂等 upsert）。
// 返回生成/更新的记录条数。
func (s *Service) GenerateDailyStats(ctx context.Context, bizDate time.Time) (int64, error) {
	start := time.Date(bizDate.Year(), bizDate.Month(), bizDate.Day(), 0, 0, 0, 0, time.Local)
	end := start.AddDate(0, 0, 1)
	sql := `
INSERT INTO t_store_daily_stats
  (merchant_id, store_id, biz_date, order_count, paid_amount, channel_breakdown, status_breakdown, generated_at)
SELECT
  o.merchant_id,
  o.store_id,
  DATE(o.paid_at) AS biz_date,
  SUM(CASE WHEN o.status='PAID' THEN 1 ELSE 0 END) AS order_count,
  SUM(CASE WHEN o.status='PAID' THEN o.paid_amount ELSE 0 END) AS paid_amount,
  JSON_OBJECT(
    'WECHAT', JSON_OBJECT(
      'count', SUM(CASE WHEN o.channel='WECHAT' AND o.status='PAID' THEN 1 ELSE 0 END),
      'amount', SUM(CASE WHEN o.channel='WECHAT' AND o.status='PAID' THEN o.paid_amount ELSE 0 END)
    ),
    'ALIPAY', JSON_OBJECT(
      'count', SUM(CASE WHEN o.channel='ALIPAY' AND o.status='PAID' THEN 1 ELSE 0 END),
      'amount', SUM(CASE WHEN o.channel='ALIPAY' AND o.status='PAID' THEN o.paid_amount ELSE 0 END)
    ),
    'OTHER', JSON_OBJECT(
      'count', SUM(CASE WHEN o.channel NOT IN ('WECHAT','ALIPAY') AND o.status='PAID' THEN 1 ELSE 0 END),
      'amount', SUM(CASE WHEN o.channel NOT IN ('WECHAT','ALIPAY') AND o.status='PAID' THEN o.paid_amount ELSE 0 END)
    )
  ) AS channel_breakdown,
  JSON_OBJECT(
    'PAID',     SUM(CASE WHEN o.status='PAID' THEN 1 ELSE 0 END),
    'REFUNDED', SUM(CASE WHEN o.status='REFUNDED' THEN 1 ELSE 0 END),
    'CLOSED',   SUM(CASE WHEN o.status='CLOSED' THEN 1 ELSE 0 END)
  ) AS status_breakdown,
  CURRENT_TIMESTAMP(3)
FROM t_order o
INNER JOIN t_store s ON s.id = o.store_id AND s.status = 1
WHERE o.paid_at >= ? AND o.paid_at < ?
GROUP BY o.merchant_id, o.store_id, DATE(o.paid_at)
ON DUPLICATE KEY UPDATE
  merchant_id = VALUES(merchant_id),
  order_count = VALUES(order_count),
  paid_amount = VALUES(paid_amount),
  channel_breakdown = VALUES(channel_breakdown),
  status_breakdown = VALUES(status_breakdown),
  generated_at = CURRENT_TIMESTAMP(3)`
	res := s.db.WithContext(ctx).Exec(sql, start, end)
	if res.Error != nil {
		return 0, res.Error
	}
	return res.RowsAffected, nil
}

// Backfill 补跑 [start, end] 区间内每一天的门店日报（含两端），供首次上线/漏跑补数。
// 返回累计生成/更新的记录条数。
func (s *Service) Backfill(ctx context.Context, start, end time.Time) (int64, error) {
	start = time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.Local)
	end = time.Date(end.Year(), end.Month(), end.Day(), 0, 0, 0, 0, time.Local)
	var total int64
	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		n, err := s.GenerateDailyStats(ctx, d)
		if err != nil {
			return total, err
		}
		total += n
	}
	return total, nil
}

// HasMissing 检查 [start, end) 区间内是否存在缺失的日报（供 Prechecker 自动补跑）。
func (s *Service) HasMissing(ctx context.Context, merchantID uint64, start, end time.Time) (bool, error) {
	return s.repo.HasMissingInRange(ctx, merchantID, start, end)
}

// RecomputeSplitStatus 异步汇总门店×日分账状态。
func (s *Service) RecomputeSplitStatus(ctx context.Context, merchantID uint64, bizDate time.Time) (int, error) {
	return s.repo.RecomputeSplitStatus(ctx, merchantID, bizDate)
}

// ResetSplitStatus 管理端重置单门店×日分账状态。
func (s *Service) ResetSplitStatus(ctx context.Context, merchantID, storeID uint64, bizDate time.Time) (bool, error) {
	return s.repo.ResetSplitStatus(ctx, merchantID, storeID, bizDate)
}

// 报表查询字段（DTO）。

// DailyItem 单门店每日聚合行。
type DailyItem struct {
	BizDate          string `json:"biz_date"`
	OrderCount       int    `json:"order_count"`
	PaidAmount       int64  `json:"paid_amount"`
	ChannelBreakdown any    `json:"channel_breakdown,omitempty"`
	StatusBreakdown  any    `json:"status_breakdown,omitempty"`
}

// StoreSummaryRow 多日范围门店汇总行。
type StoreSummaryRow struct {
	StoreID    uint64 `json:"store_id"`
	StoreName  string `json:"store_name"`
	OrderCount int    `json:"order_count"`
	PaidAmount int64  `json:"paid_amount"`
}

// ListStats 分页查询门店日报行（admin 列表）。
func (s *Service) ListStats(ctx context.Context, merchantID uint64, storeID *uint64, start, end time.Time, page, size int) ([]StoreDailyStatsModel, int64, error) {
	if page < 1 {
		page = 1
	}
	if size <= 0 || size > 200 {
		size = 20
	}
	return s.repo.ListByMerchantDateRange(ctx, merchantID, start, end, storeID, (page-1)*size, size)
}

// Summary 多日范围按门店汇总（含每日明细下钻）。
func (s *Service) Summary(ctx context.Context, merchantID uint64, storeID *uint64, start, end time.Time) (*statsrepo.StoreStatsSummaryItem, []StoreSummaryRow, error) {
	rows, err := s.repo.SummaryByDateRange(ctx, merchantID, start, end, storeID)
	if err != nil {
		return nil, nil, err
	}
	names := s.loadStoreNames(ctx, merchantID, rows)
	summary := &statsrepo.StoreStatsSummaryItem{} // 若按门店过滤则返回该门店汇总，否则全部合计
	out := make([]StoreSummaryRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, StoreSummaryRow{
			StoreID:    r.StoreID,
			StoreName:  names[r.StoreID],
			OrderCount: r.OrderCount,
			PaidAmount: r.PaidAmount,
		})
		summary.OrderCount += r.OrderCount
		summary.PaidAmount += r.PaidAmount
	}
	return summary, out, nil
}

// GetStoreDailyStats 单门店按日明细。merchantID>0 时校验门店归属该商户（admin 传 0 不过滤）。
func (s *Service) GetStoreDailyStats(ctx context.Context, merchantID, storeID uint64, start, end time.Time) ([]DailyItem, error) {
	rows, err := s.repo.ListByStoreDateRange(ctx, merchantID, storeID, start, end)
	if err != nil {
		return nil, err
	}
	out := make([]DailyItem, 0, len(rows))
	for _, r := range rows {
		out = append(out, DailyItem{
			BizDate:          r.BizDate.Format("2006-01-02"),
			OrderCount:       r.OrderCount,
			PaidAmount:       r.PaidAmount,
			ChannelBreakdown: parseJSON(r.ChannelBreakdown),
			StatusBreakdown:  parseJSON(r.StatusBreakdown),
		})
	}
	return out, nil
}

// loadStoreNames 批量加载门店名称。
func (s *Service) loadStoreNames(ctx context.Context, merchantID uint64, rows []statsrepo.StoreStatsSummaryItem) map[uint64]string {
	out := make(map[uint64]string)
	if len(rows) == 0 {
		return out
	}
	ids := make([]uint64, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.StoreID)
	}
	var names []struct {
		ID   uint64
		Name string
	}
	q := s.db.WithContext(ctx).Table("t_store").
		Select("id, name").Where("id IN ? AND deleted_at IS NULL", ids)
	if merchantID > 0 {
		q = q.Where("merchant_id = ?", merchantID)
	}
	if err := q.Scan(&names).Error; err != nil {
		s.logger.Warn("load store names fail", zap.Error(err))
		return out
	}
	for _, n := range names {
		out[n.ID] = n.Name
	}
	return out
}

// parseJSON 安全解析 JSON 列。
func parseJSON(s string) any {
	if s == "" {
		return nil
	}
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		return s
	}
	return v
}