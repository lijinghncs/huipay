-- 0017_add_t_order_store.up.sql
-- 订单记录来源门店
ALTER TABLE t_order
  ADD COLUMN store_id BIGINT UNSIGNED NULL COMMENT '关联门店 ID' AFTER merchant_id,
  ADD KEY idx_store (store_id);