-- 0002_create_t_wallet.up.sql
-- 钱包表
CREATE TABLE t_wallet (
  id              BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  wallet_no       CHAR(32)        NOT NULL COMMENT '钱包号',
  entity_id       BIGINT UNSIGNED NOT NULL,
  entity_type     VARCHAR(32)     NOT NULL,
  currency        CHAR(3)         NOT NULL DEFAULT 'CNY',
  balance         BIGINT          NOT NULL DEFAULT 0 COMMENT '可用余额（分）',
  frozen          BIGINT          NOT NULL DEFAULT 0 COMMENT '冻结余额',
  pre_frozen      BIGINT          NOT NULL DEFAULT 0 COMMENT '预冻结（分账执行中）',
  version         BIGINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '乐观锁',
  status          TINYINT         NOT NULL DEFAULT 1,
  created_at      DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at      DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  UNIQUE KEY uk_wallet_no (wallet_no),
  UNIQUE KEY uk_entity_currency (entity_id, currency)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='钱包表';