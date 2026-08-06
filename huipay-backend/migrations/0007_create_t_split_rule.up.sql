-- 0007_create_t_split_rule.up.sql
-- 分账规则表
CREATE TABLE t_split_rule (
  id              BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  rule_code       CHAR(32)        NOT NULL,
  merchant_id     BIGINT UNSIGNED NOT NULL,
  rule_name       VARCHAR(128)    NOT NULL,
  priority        INT             NOT NULL DEFAULT 0,
  conditions      JSON            NOT NULL,
  allocations     JSON            NOT NULL,
  trigger_type    VARCHAR(32)     NOT NULL DEFAULT 'PAID',
  status          TINYINT         NOT NULL DEFAULT 1,
  effective_from  DATETIME(3)     NULL,
  effective_to    DATETIME(3)     NULL,
  created_at      DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at      DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  UNIQUE KEY uk_rule_code (rule_code),
  KEY idx_merchant_priority (merchant_id, priority DESC)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='分账规则表';

CREATE TABLE t_split_execution (
  id                 CHAR(20)        NOT NULL,
  order_no           CHAR(32)        NOT NULL,
  rule_id            BIGINT UNSIGNED NULL,
  receiver_entity_id BIGINT UNSIGNED NOT NULL,
  receiver_type      VARCHAR(32)     NOT NULL,
  amount             BIGINT          NOT NULL,
  level              INT             NOT NULL DEFAULT 1,
  channel            VARCHAR(32)     NULL,
  channel_split_no   VARCHAR(64)     NULL,
  status             VARCHAR(16)     NOT NULL DEFAULT 'PENDING',
  retry_count        INT             NOT NULL DEFAULT 0,
  last_error         VARCHAR(512)    NULL,
  executed_at        DATETIME(3)     NULL,
  created_at         DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at         DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  UNIQUE KEY uk_order_receiver (order_no, receiver_entity_id),
  KEY idx_status_executed (status, executed_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='分账执行记录';