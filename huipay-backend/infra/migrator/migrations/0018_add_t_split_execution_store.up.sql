-- 0018_add_t_split_execution_store.up.sql
-- 分账执行记录带出门店，便于排查
ALTER TABLE t_split_execution
  ADD COLUMN store_id BIGINT UNSIGNED NULL COMMENT '关联门店 ID' AFTER order_no,
  ADD KEY idx_store (store_id);