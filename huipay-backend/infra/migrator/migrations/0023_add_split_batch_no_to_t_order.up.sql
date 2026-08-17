-- 0023_add_split_batch_no_to_t_order.up.sql
-- 分账批次号落订单：供「门店订单明细」按批次号关联查询，替代 order_no IN (order_nos) 大列表反查。

-- 1) t_order 加分账批次号 + 索引（merchant_id 前缀 + 批次号 + 门店，支撑分账明细查询）
ALTER TABLE t_order
  ADD COLUMN split_batch_no VARCHAR(64) NULL COMMENT '所属分账批次号(SP{rule}-{start}-{end})' AFTER split_status,
  ADD KEY idx_split_batch (merchant_id, split_batch_no, store_id);

-- 2) 回填历史数据：将未驳回分账单(order_nos 覆盖)的订单回填批次号
UPDATE t_order o
JOIN t_split_bill sb
  ON sb.status IN ('PENDING','APPROVED','EXECUTED')
 AND sb.order_nos IS NOT NULL
 AND JSON_CONTAINS(sb.order_nos, JSON_QUOTE(o.order_no))
SET o.split_batch_no = sb.batch_no
WHERE o.split_batch_no IS NULL;