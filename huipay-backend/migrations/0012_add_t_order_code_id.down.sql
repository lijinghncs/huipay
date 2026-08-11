-- 0012_add_t_order_code_id.down.sql
ALTER TABLE t_order
  DROP INDEX idx_order_code,
  DROP COLUMN code_id;