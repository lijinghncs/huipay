-- 0023_add_split_batch_no_to_t_order.down.sql
ALTER TABLE t_order
  DROP INDEX idx_split_batch,
  DROP COLUMN split_batch_no;