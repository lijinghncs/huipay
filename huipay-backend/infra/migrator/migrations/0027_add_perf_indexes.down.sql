-- 0027_add_perf_indexes.down.sql
-- 回滚性能索引

ALTER TABLE t_order
  DROP KEY idx_store_paidat,
  DROP KEY idx_merchant_split,
  DROP KEY idx_merchant_status_paidat;