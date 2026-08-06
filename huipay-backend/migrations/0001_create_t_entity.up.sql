-- 0001_create_t_entity.up.sql
-- 主体表：商户 / 门店 / 推广员 / 平台 / ISV
CREATE TABLE t_entity (
  id              BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  entity_code     CHAR(32)        NOT NULL COMMENT '对外主体号',
  entity_type     VARCHAR(32)     NOT NULL COMMENT 'MERCHANT/STORE/PROMOTER/PLATFORM/ISV',
  parent_id       BIGINT UNSIGNED NULL COMMENT '上级主体',
  name            VARCHAR(128)    NOT NULL,
  kyc_status      TINYINT         NOT NULL DEFAULT 0,
  kyc_data        JSON            NULL,
  status          TINYINT         NOT NULL DEFAULT 1,
  created_at      DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at      DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  deleted_at      DATETIME(3)     NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_entity_code (entity_code),
  KEY idx_parent (parent_id),
  KEY idx_type_status (entity_type, status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='主体表';