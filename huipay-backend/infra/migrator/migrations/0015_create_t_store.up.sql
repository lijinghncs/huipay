-- 0015_create_t_store.up.sql
-- 门店表：商户的子资源，独立存储，不复用 t_entity
CREATE TABLE t_store (
  id              BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  store_code      CHAR(32)        NOT NULL COMMENT '门店编码（系统生成，前缀 S）',
  merchant_id     BIGINT UNSIGNED NOT NULL COMMENT '所属商户主体 ID',
  name            VARCHAR(64)     NOT NULL COMMENT '门店名称',
  store_type      VARCHAR(16)     NULL     COMMENT '门店类型：DIRECT/FRANCHISE/PARTNER',
  contact_phone   VARCHAR(32)     NULL     COMMENT '联系电话',
  region          VARCHAR(128)    NULL     COMMENT '所在地区（省/市/区）',
  address         VARCHAR(256)    NULL     COMMENT '详细地址',
  longitude       DECIMAL(10, 6)  NULL     COMMENT '经度（预留地图可视化）',
  latitude        DECIMAL(10, 6)  NULL     COMMENT '纬度（预留地图可视化）',
  metadata        JSON            NULL     COMMENT '扩展元数据（营业时间/标签等）',
  status          TINYINT         NOT NULL DEFAULT 1 COMMENT '1=启用 0=停用',
  created_at      DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at      DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  deleted_at      DATETIME(3)     NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_store_code (store_code),
  KEY idx_merchant_status (merchant_id, status, deleted_at),
  KEY idx_name (merchant_id, name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='门店表';

-- 门店审计日志
CREATE TABLE t_store_audit_log (
  id            BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  store_id      BIGINT UNSIGNED NOT NULL,
  merchant_id   BIGINT UNSIGNED NOT NULL,
  action        VARCHAR(32)     NOT NULL COMMENT 'CREATE/UPDATE/STATUS/DELETE/CODE_LINK',
  operator      VARCHAR(64)     NULL,
  detail        JSON            NULL,
  created_at    DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  KEY idx_store (store_id, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='门店审计日志';