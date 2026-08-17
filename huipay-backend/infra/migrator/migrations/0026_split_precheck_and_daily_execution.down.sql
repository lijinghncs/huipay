-- 0026_split_precheck_and_daily_execution.down.sql
-- 回滚：分账前置对账 + 每日执行记录（V2 合并版）

-- 1) t_store_daily_stats 移除分账字段
ALTER TABLE t_store_daily_stats
  DROP KEY idx_split_status,
  DROP COLUMN split_total_amount,
  DROP COLUMN split_at,
  DROP COLUMN split_batch_no,
  DROP COLUMN split_status;

-- 2) t_split_daily_execution
DROP TABLE IF EXISTS t_split_daily_execution;

-- 3) t_split_bill_biz_date
DROP TABLE IF EXISTS t_split_bill_biz_date;

-- 4) t_split_bill 移除 biz_dates
ALTER TABLE t_split_bill DROP COLUMN biz_dates;

-- 5) t_reconcile_diff 移除列与索引
ALTER TABLE t_reconcile_diff
  DROP KEY idx_merchant_biz_date,
  DROP KEY idx_diff_type_biz_date,
  DROP COLUMN merchant_id;

-- 6) t_split_audit 移除补充索引
ALTER TABLE t_split_audit
  DROP KEY idx_action_time_status,
  DROP KEY idx_biz_time;