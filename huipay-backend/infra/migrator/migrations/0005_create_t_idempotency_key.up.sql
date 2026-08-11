-- 0005_create_t_idempotency_key.up.sql
-- 幂等键中心
CREATE TABLE t_idempotency_key (
  id                BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  scope             VARCHAR(64)     NOT NULL,
  idempotency_key   VARCHAR(64)     NOT NULL,
  request_hash      CHAR(64)        NOT NULL,
  response_snapshot JSON            NULL,
  status            TINYINT         NOT NULL DEFAULT 1,
  expire_at         DATETIME(3)     NOT NULL,
  created_at        DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  UNIQUE KEY uk_scope_key (scope, idempotency_key),
  KEY idx_expire (expire_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='幂等键中心';