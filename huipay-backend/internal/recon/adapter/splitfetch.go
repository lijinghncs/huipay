// Package adapter 提供 recon 域的 gorm 取数适配器：
// SQL 口径从原 split/scope 与 split/scheduler 原样平移，行为不变。
package adapter

import (
	"context"
	"time"

	"gorm.io/gorm"

	"github.com/huipay/huipay-backend/internal/recon/domain"
)

// SplitFetcher 分账侧取数适配器（执行后对账：执行记录 + 账本）。
type SplitFetcher struct {
	db *gorm.DB
}

func NewSplitFetcher(db *gorm.DB) *SplitFetcher {
	return &SplitFetcher{db: db}
}

// ListMerchantsWithExecution 当日有 SUCCESS 执行记录的商户列表。
func (f *SplitFetcher) ListMerchantsWithExecution(ctx context.Context, start, end time.Time) ([]uint64, error) {
	var ids []uint64
	err := f.db.WithContext(ctx).Raw(`
		SELECT DISTINCT sos.merchant_id
		FROM t_split_execution se
		JOIN t_split_order_status sos ON sos.order_no = se.order_no
		WHERE se.executed_at >= ? AND se.executed_at < ?
		  AND se.status = 'SUCCESS'
		  AND sos.status = 'SUCCESS'
	`, start, end).Scan(&ids).Error
	return ids, err
}

// SumByOrder 当日 SUCCESS 执行记录按订单聚合（金额合计 + 是否降级）。
func (f *SplitFetcher) SumByOrder(ctx context.Context, merchantID uint64, start, end time.Time) ([]domain.OrderExecSum, error) {
	var rows []domain.OrderExecSum
	err := f.db.WithContext(ctx).Raw(`
		SELECT se.order_no AS order_no,
		       SUM(se.amount) AS exec_sum,
		       MAX(se.degraded) AS degraded
		FROM t_split_execution se
		JOIN t_split_order_status sos ON sos.order_no = se.order_no
		WHERE se.executed_at >= ? AND se.executed_at < ?
		  AND se.status = 'SUCCESS'
		  AND sos.merchant_id = ?
		  AND sos.status = 'SUCCESS'
		GROUP BY se.order_no
	`, start, end, merchantID).Scan(&rows).Error
	return rows, err
}

// SumByOrderNos 本地账本 SPLIT CREDIT 入账合计（按订单号）。
func (f *SplitFetcher) SumByOrderNos(ctx context.Context, orderNos []string) (map[string]int64, error) {
	if len(orderNos) == 0 {
		return map[string]int64{}, nil
	}
	type row struct {
		BizID string `gorm:"column:biz_id"`
		Sum   int64  `gorm:"column:sum"`
	}
	var rows []row
	err := f.db.WithContext(ctx).Raw(`
		SELECT biz_id, SUM(amount) AS sum
		FROM t_journal_entry
		WHERE biz_type = 'SPLIT' AND direction = 'CREDIT'
		  AND biz_id IN (?)
		GROUP BY biz_id
	`, orderNos).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make(map[string]int64, len(rows))
	for _, r := range rows {
		out[r.BizID] = r.Sum
	}
	return out, nil
}
