-- 0004_create_t_order.up.sql
-- 订单主表
CREATE TABLE t_order (
  id                  BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  order_no            CHAR(32)        NOT NULL COMMENT '平台订单号',
  merchant_order_no   VARCHAR(64)     NOT NULL,
  merchant_id         BIGINT UNSIGNED NOT NULL,
  parent_order_no     CHAR(32)        NULL,
  order_type          VARCHAR(32)     NOT NULL DEFAULT 'PAYMENT',
  amount              BIGINT          NOT NULL COMMENT '订单金额（分）',
  paid_amount         BIGINT          NOT NULL DEFAULT 0,
  coupon_discount     BIGINT          NOT NULL DEFAULT 0,
  channel             VARCHAR(32)     NULL,
  channel_trade_no    VARCHAR(64)     NULL,
  split_status        VARCHAR(16)     NOT NULL DEFAULT 'PENDING',
  status              VARCHAR(16)     NOT NULL DEFAULT 'CREATED',
  expire_at           DATETIME(3)     NULL,
  paid_at             DATETIME(3)     NULL,
  created_at          DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at          DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  deleted_at          DATETIME(3)     NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_order_no (order_no),
  UNIQUE KEY uk_merchant_order (merchant_id, merchant_order_no),
  KEY idx_status_created (status, created_at),
  KEY idx_channel_trade (channel, channel_trade_no)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='订单主表';