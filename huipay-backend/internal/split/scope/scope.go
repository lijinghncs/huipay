// 包 scope 提供分账/对账相关的统一口径过滤器与预编译 SQL。
//
// 所有读 t_order 的聚合查询（日报生成、Prechecker、补跑、回算 split_status）必须使用
// SameScopeFilter 或 LEFT JOIN 优化版本，保证与门店日报聚合 SQL 完全一致，避免口径错位
// 导致的对账失败（V2 评审问题 🔴2）。
package scope

import "time"

// SameScopeFilter 返回「同一聚合口径」的 WHERE 片段。
//
// 口径定义：
//   - PAID 状态、deleted_at IS NULL
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

// HasMissingQuery 检查 [from, to) 区间内是否存在缺失 biz_date 的 SQL。
// 返回一个 0/1：1 表示区间内有缺失或日报行数与期望不一致。
const HasMissingQuery = `
SELECT COUNT(*) AS missing_days
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
  LEFT JOIN t_store_daily_stats s
    ON s.merchant_id = ? AND s.biz_date = DATE(d.day) AND s.paid_amount > 0
  WHERE s.id IS NULL
) m`