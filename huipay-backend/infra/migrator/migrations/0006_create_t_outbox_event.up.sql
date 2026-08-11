-- 0006_create_t_outbox_event.up.sql
-- 本地消息表（替代 Kafka）
CREATE TABLE t_outbox_event (
  id             CHAR(20)        NOT NULL,
  aggregate_type VARCHAR(64)     NOT NULL,
  aggregate_id   VARCHAR(64)     NOT NULL,
  event_type     VARCHAR(64)     NOT NULL,
  payload        JSON            NOT NULL,
  status         VARCHAR(16)     NOT NULL DEFAULT 'PENDING',
  retry_count    INT             NOT NULL DEFAULT 0,
  next_retry_at  DATETIME(3)     NULL,
  processed_at   DATETIME(3)     NULL,
  created_at     DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  KEY idx_status_retry (status, next_retry_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='本地消息表';