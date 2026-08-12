-- 0017_add_t_order_store.down.sql
ALTER TABLE t_order
  DROP KEY idx_store,
  DROP COLUMN store_id;