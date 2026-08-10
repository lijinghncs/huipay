-- 0009_create_t_reconcile_diff.up.sql
-- 对账差异表：T+1 对账发现的长款/短款/金额不一致落库，供人工排查与后续告警。
CREATE TABLE IF NOT EXISTS t_reconcile_diff (
  id              BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  biz_date        DATE            NOT NULL,
  diff_type       VARCHAR(16)     NOT NULL COMMENT 'LONG/SHORT/MISMATCH',
  order_no        CHAR(32)        NULL,
  transaction_id  VARCHAR(64)     NULL,
  local_amount    BIGINT          NULL,
  channel_amount  BIGINT          NULL,
  detail          JSON            NULL,
  resolved_at     DATETIME(3)     NULL,
  created_at      DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  KEY idx_date_type (biz_date, diff_type),
  KEY idx_order (order_no)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='对账差异表';