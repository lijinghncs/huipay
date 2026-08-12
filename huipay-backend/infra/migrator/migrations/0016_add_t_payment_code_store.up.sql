-- 0016_add_t_payment_code_store.up.sql
-- 收款码牌关联门店（软约束：允许 NULL，兼容历史数据）
ALTER TABLE t_payment_code
  ADD COLUMN store_id BIGINT UNSIGNED NULL COMMENT '关联门店 ID' AFTER merchant_id,
  ADD KEY idx_store (store_id);