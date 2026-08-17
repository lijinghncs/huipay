// Package adapter 提供对账各侧数据的 gorm 取数适配器。
//
// 本文件承载分账对账口径 SQL（迁自 split/scope，口径逐字保留）：
// 订单侧与日报侧必须使用同一过滤口径，否则对账结果无意义。
package adapter

import "time"

// SameScopeFilter 返回"分账口径"订单过滤条件与参数（用于单测断言与未来扩展）。
//
// 分账口径（与 internal/split/repository/store_revenue_repo.go 的 splitExclusion 对齐）：
//   - merchant_id 匹配、PAID 状态、deleted_at IS NULL
//   - paid_at ∈ [from, to)
//   - store_id NOT NULL 且 t_store.status = 1（门店未删除）
//   - 排除 t_split_execution.status = 'SUCCESS'（已成功分账）
//   - 排除 t_split_bill_biz_date 覆盖的订单（账单已 EXECUTED）
//
// 注意：本函数返回的 NOT EXISTS 三层嵌套子查询性能较差（V2 评审问题 🔴7），
// 实际执行路径请使用 Prechecker 中的 LEFT JOIN 优化版 SQL。
// 本函数保留用于单元测试断言与未来扩展场景。
func SameScopeFilter(merchantID uint64, from, to time.Time) (where string, args []any) {
	where = `
        o.merchant_id = ? AND o.status = 'PAID' AND o.deleted_at IS NULL
        AND o.paid_at >= ? AND o.paid_at < ?
        AND o.store_id IS NOT NULL
        AND EXISTS (SELECT 1 FROM t_store s WHERE s.id = o.store_id AND s.status = 1)
        AND NOT EXISTS (SELECT 1 FROM t_split_execution se
                        WHERE se.order_no = o.order_no AND se.status = 'SUCCESS')
        AND NOT EXISTS (SELECT 1 FROM t_split_bill sb
                        WHERE sb.merchant_id = o.merchant_id
                          AND sb.status = 'EXECUTED'
                          AND sb.id IN (
                              SELECT bill_id FROM t_split_bill_biz_date
                              WHERE biz_date = DATE(o.paid_at)
                          ))
    `
	args = []any{merchantID, from, to}
	return
}

// LayerAQuery Prechecker Layer A 优化版 SQL（商户级实收合计）。
// LEFT JOIN + 物化，单次评估 O(N)；配合索引 idx_merchant_status_paidat 性能最佳。
// 调用方需用 USE INDEX hint 或确认优化器已选择正确索引。
const LayerAQuery = `
SELECT COALESCE(SUM(o.paid_amount), 0) AS order_total
FROM t_order o USE INDEX (idx_merchant_status_paidat)
INNER JOIN t_store s ON s.id = o.store_id AND s.status = 1
LEFT JOIN t_split_execution se ON se.order_no = o.order_no AND se.status = 'SUCCESS'
LEFT JOIN t_split_bill sb ON sb.merchant_id = o.merchant_id AND sb.status = 'EXECUTED'
LEFT JOIN t_split_bill_biz_date bd
       ON bd.bill_id = sb.id AND bd.biz_date = DATE(o.paid_at)
WHERE o.merchant_id = ?
  AND o.status = 'PAID' AND o.deleted_at IS NULL
  AND o.paid_at >= ? AND o.paid_at < ?
  AND o.store_id IS NOT NULL
  AND se.order_no IS NULL
  AND bd.bill_id IS NULL`

// LayerBQuery Prechecker Layer B 优化版 SQL（门店×日实收合计）。
const LayerBQuery = `
SELECT DATE(o.paid_at) AS biz_date,
       o.store_id,
       COALESCE(SUM(o.paid_amount), 0) AS order_total
FROM t_order o USE INDEX (idx_merchant_status_paidat)
INNER JOIN t_store s ON s.id = o.store_id AND s.status = 1
LEFT JOIN t_split_execution se ON se.order_no = o.order_no AND se.status = 'SUCCESS'
LEFT JOIN t_split_bill sb ON sb.merchant_id = o.merchant_id AND sb.status = 'EXECUTED'
LEFT JOIN t_split_bill_biz_date bd
       ON bd.bill_id = sb.id AND bd.biz_date = DATE(o.paid_at)
WHERE o.merchant_id = ?
  AND o.status = 'PAID' AND o.deleted_at IS NULL
  AND o.paid_at >= ? AND o.paid_at < ?
  AND o.store_id IS NOT NULL
  AND se.order_no IS NULL
  AND bd.bill_id IS NULL
GROUP BY DATE(o.paid_at), o.store_id`

// StatsSumQuery 门店日报合计 SQL（用于 Prechecker 对比侧）。
const StatsSumQuery = `
SELECT COALESCE(SUM(paid_amount), 0) AS stats_total
FROM t_store_daily_stats
WHERE merchant_id = ?
  AND biz_date >= ? AND biz_date < ?`

// StatsRowsQuery 门店日报明细 SQL（用于 Prechecker 对比侧）。
const StatsRowsQuery = `
SELECT biz_date, store_id, paid_amount AS stats_total
FROM t_store_daily_stats
WHERE merchant_id = ?
  AND biz_date >= ? AND biz_date < ?`
