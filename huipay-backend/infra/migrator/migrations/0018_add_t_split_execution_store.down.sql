-- 0018_add_t_split_execution_store.down.sql
ALTER TABLE t_split_execution
  DROP KEY idx_store,
  DROP COLUMN store_id;