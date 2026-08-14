-- 0022_split_resilience.down.sql
DROP TABLE IF EXISTS t_split_audit;
ALTER TABLE t_split_execution
  DROP INDEX idx_channel_req,
  DROP COLUMN degraded,
  DROP COLUMN channel_req_no;
DROP TABLE IF EXISTS t_split_order_status;