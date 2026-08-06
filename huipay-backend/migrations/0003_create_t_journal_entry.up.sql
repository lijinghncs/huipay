-- 0003_create_t_journal_entry.up.sql
-- 账本流水（不可变）
CREATE TABLE t_journal_entry (
  id               CHAR(20)        NOT NULL COMMENT '雪花 ID',
  wallet_id        BIGINT UNSIGNED NOT NULL,
  direction        ENUM('DEBIT','CREDIT') NOT NULL,
  amount           BIGINT          NOT NULL COMMENT '金额（分）',
  balance_after    BIGINT          NOT NULL COMMENT '操作后可用余额',
  biz_type         VARCHAR(32)     NOT NULL COMMENT 'PAYMENT/SPLIT/REFUND/WITHDRAW/ADJUST/FREEZE/UNFREEZE',
  biz_id           VARCHAR(64)     NOT NULL,
  counterparty_id  BIGINT UNSIGNED NULL,
  idempotency_key  VARCHAR(64)     NOT NULL,
  trace_id         VARCHAR(64)     NULL,
  remark           VARCHAR(255)    NULL,
  created_at       DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  UNIQUE KEY uk_idem (wallet_id, idempotency_key),
  KEY idx_biz (biz_type, biz_id),
  KEY idx_wallet_created (wallet_id, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='账本流水（不可变）';