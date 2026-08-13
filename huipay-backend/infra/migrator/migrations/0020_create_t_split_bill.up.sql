-- 0020_create_t_split_bill.up.sql
-- 分账单表：按时间段生成总账单，审批通过后执行分账
CREATE TABLE t_split_bill (
  id            BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  batch_no      VARCHAR(64)     NOT NULL,
  merchant_id   BIGINT UNSIGNED NOT NULL,
  rule_code     CHAR(32)        NOT NULL,
  rule_name     VARCHAR(128)    NOT NULL,
  start_time    DATETIME(3)     NOT NULL,
  end_time      DATETIME(3)     NOT NULL,
  total_amount  BIGINT          NOT NULL COMMENT '时间段内商户实收总额(分)',
  detail        JSON            NOT NULL COMMENT '各门店可分金额明细',
  status        VARCHAR(16)     NOT NULL DEFAULT 'PENDING' COMMENT 'PENDING待审批 APPROVED已通过 REJECTED已驳回 EXECUTED已执行',
  approved_at   DATETIME(3)     NULL,
  executed_at   DATETIME(3)     NULL,
  created_at    DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at    DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  UNIQUE KEY uk_batch_no (batch_no),
  KEY idx_merchant_status (merchant_id, status, created_at DESC)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='分账单';