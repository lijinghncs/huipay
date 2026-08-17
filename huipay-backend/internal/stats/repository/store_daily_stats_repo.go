// 包 repository 提供门店订单日报数据访问。
package repository

import (
	"context"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// StoreDailyStatsModel 门店订单日报表 GORM 模型（t_store_daily_stats）。
type StoreDailyStatsModel struct {
	ID               uint64    `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	MerchantID       uint64    `gorm:"column:merchant_id;not null" json:"merchant_id"`
	StoreID          uint64    `gorm:"column:store_id;not null" json:"store_id"`
	BizDate          time.Time `gorm:"column:biz_date;type:date;not null" json:"biz_date"`
	OrderCount       int       `gorm:"column:order_count;not null;default:0" json:"order_count"`
	PaidAmount       int64     `gorm:"column:paid_amount;not null;default:0" json:"paid_amount"`
	ChannelBreakdown string    `gorm:"column:channel_breakdown;type:json" json:"channel_breakdown,omitempty"`
	StatusBreakdown  string    `gorm:"column:status_breakdown;type:json" json:"status_breakdown,omitempty"`
	// V2 分账状态字段：由后台任务根据 t_order.split_batch_no 汇总
	SplitStatus      string     `gorm:"column:split_status;default:PENDING" json:"split_status"`
	SplitBatchNo     *string    `gorm:"column:split_batch_no" json:"split_batch_no,omitempty"`
	SplitAt          *time.Time `gorm:"column:split_at" json:"split_at,omitempty"`
	SplitTotalAmount int64      `gorm:"column:split_total_amount;default:0" json:"split_total_amount"`
	GeneratedAt      time.Time  `gorm:"column:generated_at;autoCreateTime" json:"generated_at"`
	UpdatedAt        time.Time  `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
}

// TableName 表名。
func (StoreDailyStatsModel) TableName() string { return "t_store_daily_stats" }

// StoreDailyStatsRepo 门店日报仓储。
type StoreDailyStatsRepo struct{ db *gorm.DB }

// NewStoreDailyStatsRepo 构造 StoreDailyStatsRepo。
func NewStoreDailyStatsRepo(db *gorm.DB) *StoreDailyStatsRepo { return &StoreDailyStatsRepo{db: db} }

// DB 暴露底层连接。
func (r *StoreDailyStatsRepo) DB() *gorm.DB { return r.db }

// Upsert 批量幂等写入（Iuk_store_date 冲突时覆盖）。
func (r *StoreDailyStatsRepo) Upsert(ctx context.Context, rows []StoreDailyStatsModel) error {
	if len(rows) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "store_id"}, {Name: "biz_date"}},
			DoUpdates: clause.AssignmentColumns([]string{"merchant_id", "order_count", "paid_amount", "channel_breakdown", "status_breakdown", "generated_at"}),
		}).
		Create(&rows).Error
}

// ListByMerchantDateRange 分页查询（start <= biz_date < end）。merchantID=0 表示全部商户。
func (r *StoreDailyStatsRepo) ListByMerchantDateRange(ctx context.Context, merchantID uint64, start, end time.Time, storeID *uint64, offset, limit int) ([]StoreDailyStatsModel, int64, error) {
	db := r.db.WithContext(ctx).Model(&StoreDailyStatsModel{}).
		Where("biz_date >= ? AND biz_date < ?", start, end)
	if merchantID > 0 {
		db = db.Where("merchant_id = ?", merchantID)
	}
	if storeID != nil && *storeID > 0 {
		db = db.Where("store_id = ?", *storeID)
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []StoreDailyStatsModel
	if err := db.Order("biz_date DESC").Offset(offset).Limit(limit).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// StoreStatsSummaryItem 多日范围门店汇总行。
type StoreStatsSummaryItem struct {
	StoreID    uint64 `gorm:"column:store_id" json:"store_id"`
	OrderCount int    `gorm:"column:order_count" json:"order_count"`
	PaidAmount int64  `gorm:"column:paid_amount" json:"paid_amount"`
}

// SummaryByDateRange 按门店聚合多日范围汇总。merchantID=0 表示全部商户。
func (r *StoreDailyStatsRepo) SummaryByDateRange(ctx context.Context, merchantID uint64, start, end time.Time, storeID *uint64) ([]StoreStatsSummaryItem, error) {
	db := r.db.WithContext(ctx).Model(&StoreDailyStatsModel{}).
		Select("store_id, SUM(order_count) AS order_count, SUM(paid_amount) AS paid_amount").
		Where("biz_date >= ? AND biz_date < ?", start, end)
	if merchantID > 0 {
		db = db.Where("merchant_id = ?", merchantID)
	}
	if storeID != nil && *storeID > 0 {
		db = db.Where("store_id = ?", *storeID)
	}
	var rows []StoreStatsSummaryItem
	if err := db.Group("store_id").Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// ListByStoreDateRange 按门店查询每日明细。merchantID>0 时校验门店归属该商户。
func (r *StoreDailyStatsRepo) ListByStoreDateRange(ctx context.Context, merchantID, storeID uint64, start, end time.Time) ([]StoreDailyStatsModel, error) {
	db := r.db.WithContext(ctx).Model(&StoreDailyStatsModel{}).
		Where("store_id = ? AND biz_date >= ? AND biz_date < ?", storeID, start, end)
	if merchantID > 0 {
		db = db.Where("merchant_id = ?", merchantID)
	}
	var rows []StoreDailyStatsModel
	if err := db.Order("biz_date ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// HasMissingInRange 检查 [start, end) 区间内是否存在缺失的日报行（按 paid_amount>0 判定）。
func (r *StoreDailyStatsRepo) HasMissingInRange(ctx context.Context, merchantID uint64, start, end time.Time) (bool, error) {
	type row struct {
		Days int64 `gorm:"column:days"`
	}
	q := `
        SELECT COUNT(*) AS days
        FROM (
          SELECT DATE(d.day) AS biz_date FROM (
            SELECT DATE_ADD(?, INTERVAL n.n DAY) AS day
            FROM (
              SELECT 0 AS n UNION ALL SELECT 1 UNION ALL SELECT 2 UNION ALL SELECT 3 UNION ALL SELECT 4
              UNION ALL SELECT 5 UNION ALL SELECT 6 UNION ALL SELECT 7 UNION ALL SELECT 8 UNION ALL SELECT 9
              UNION ALL SELECT 10 UNION ALL SELECT 11 UNION ALL SELECT 12 UNION ALL SELECT 13 UNION ALL SELECT 14
              UNION ALL SELECT 15 UNION ALL SELECT 16 UNION ALL SELECT 17 UNION ALL SELECT 18 UNION ALL SELECT 19
              UNION ALL SELECT 20 UNION ALL SELECT 21 UNION ALL SELECT 22 UNION ALL SELECT 23 UNION ALL SELECT 24
              UNION ALL SELECT 25 UNION ALL SELECT 26 UNION ALL SELECT 27 UNION ALL SELECT 28 UNION ALL SELECT 29
            ) n
            WHERE DATE_ADD(?, INTERVAL n.n DAY) < ?
          ) d
        ) expected
        LEFT JOIN t_store_daily_stats s
          ON s.merchant_id = ? AND s.biz_date = expected.biz_date AND s.paid_amount > 0
        WHERE s.id IS NULL`
	var r0 row
	if err := r.db.WithContext(ctx).Raw(q, start, start, end, merchantID).Scan(&r0).Error; err != nil {
		return false, err
	}
	return r0.Days > 0, nil
}

// RecomputeSplitStatus 异步汇总：从 t_order.split_batch_no 反算 split_status。
// 设计要点：
//   - 仅写「已分账比例」非 100% 的门店（避免无效 UPDATE）
//   - 已分账比例 = 100% → split_status='SUCCESS'，split_batch_no 取最近一次执行
//   - 已分账比例 = 0%   → split_status='PENDING'，清空 split_batch_no/split_at
//   - 其他               → split_status='PARTIAL'
//
// 返回影响的门店×日数。
func (r *StoreDailyStatsRepo) RecomputeSplitStatus(ctx context.Context, merchantID uint64, bizDate time.Time) (int, error) {
	// 1. 统计每个门店的已分账金额 / 总金额
	type row struct {
		StoreID       uint64 `gorm:"column:store_id"`
		TotalAmount   int64  `gorm:"column:total_amount"`
		SplitAmount   int64  `gorm:"column:split_amount"`
		LatestBatchNo string `gorm:"column:latest_batch_no"`
	}
	q := `
        SELECT o.store_id,
               COALESCE(SUM(o.paid_amount), 0) AS total_amount,
               COALESCE(SUM(CASE WHEN o.split_batch_no IS NOT NULL AND o.split_batch_no <> ''
                                 THEN o.paid_amount ELSE 0 END), 0) AS split_amount,
               SUBSTRING_INDEX(GROUP_CONCAT(CASE WHEN o.split_batch_no IS NOT NULL
                                                 THEN o.split_batch_no END
                                                 ORDER BY o.paid_at DESC SEPARATOR ','), ',', 1) AS latest_batch_no
        FROM t_order o USE INDEX (idx_merchant_split)
        INNER JOIN t_store s ON s.id = o.store_id AND s.status = 1
        WHERE o.merchant_id = ?
          AND o.status = 'PAID' AND o.deleted_at IS NULL
          AND DATE(o.paid_at) = ?
        GROUP BY o.store_id`
	var rows []row
	if err := r.db.WithContext(ctx).Raw(q, merchantID, bizDate.Format("2006-01-02")).Scan(&rows).Error; err != nil {
		return 0, err
	}

	updated := 0
	now := time.Now()
	for _, row := range rows {
		var status string
		var batchNo *string
		var splitAt *time.Time
		var splitTotal int64
		switch {
		case row.TotalAmount == 0:
			continue
		case row.SplitAmount == 0:
			status = "PENDING"
		case row.SplitAmount >= row.TotalAmount:
			status = "SUCCESS"
			batchNo = &row.LatestBatchNo
			splitAt = &now
			splitTotal = row.SplitAmount
		default:
			status = "PARTIAL"
			batchNo = &row.LatestBatchNo
			splitAt = &now
			splitTotal = row.SplitAmount
		}
		res := r.db.WithContext(ctx).Model(&StoreDailyStatsModel{}).
			Where("merchant_id = ? AND biz_date = ? AND store_id = ?", merchantID, bizDate.Format("2006-01-02"), row.StoreID).
			Updates(map[string]any{
				"split_status":       status,
				"split_batch_no":     batchNo,
				"split_at":           splitAt,
				"split_total_amount": splitTotal,
			})
		if res.Error != nil {
			return updated, res.Error
		}
		updated += int(res.RowsAffected)
	}
	return updated, nil
}

// ResetSplitStatus 管理端重置：将门店×日分账状态置 PENDING，清空批次号。
func (r *StoreDailyStatsRepo) ResetSplitStatus(ctx context.Context, merchantID, storeID uint64, bizDate time.Time) (bool, error) {
	res := r.db.WithContext(ctx).Model(&StoreDailyStatsModel{}).
		Where("merchant_id = ? AND store_id = ? AND biz_date = ?", merchantID, storeID, bizDate.Format("2006-01-02")).
		Updates(map[string]any{
			"split_status":       "PENDING",
			"split_batch_no":     nil,
			"split_at":           nil,
			"split_total_amount": 0,
		})
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
}