-- 0016_add_t_payment_code_store.down.sql
ALTER TABLE t_payment_code
  DROP KEY idx_store,
  DROP COLUMN store_id;