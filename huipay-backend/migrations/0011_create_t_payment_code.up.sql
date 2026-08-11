-- 0011_create_t_payment_code.up.sql
-- 收款码牌表：商户收款码牌，供消费者扫码进入收银台
CREATE TABLE t_payment_code (
  id          BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  merchant_id BIGINT UNSIGNED NOT NULL COMMENT '所属商户主体 ID',
  code_id     VARCHAR(16)     NOT NULL COMMENT '对外短码（6 位字母数字，排除歧义字符）',
  status      TINYINT         NOT NULL DEFAULT 1 COMMENT '1=启用 0=停用',
  remark      VARCHAR(64)     NULL COMMENT '备注（门店/场景名）',
  created_at  DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  disabled_at DATETIME(3)     NULL COMMENT '停用时间',
  deleted_at  DATETIME(3)     NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_code_id (code_id),
  KEY idx_merchant_status (merchant_id, status, deleted_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='收款码牌表';