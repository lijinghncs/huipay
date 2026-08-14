-- 0022_split_resilience.up.sql
-- 分账容错迭代：订单级状态表 + 审计表 + 执行记录增列（通道幂等单号/降级标记）

-- 1) 订单级分账状态表（悬挂检测/补偿重试调度/列表查询）
CREATE TABLE t_split_order_status (
  order_no        CHAR(64)     NOT NULL,
  merchant_id     BIGINT UNSIGNED NOT NULL,
  rule_id         BIGINT UNSIGNED NULL,
  rule_snapshot   JSON         NULL COMMENT '执行时分配快照',
  total_amount    BIGINT       NOT NULL,
  receiver_count  INT          NOT NULL DEFAULT 0,
  success_count   INT          NOT NULL DEFAULT 0,
  status          VARCHAR(16)  NOT NULL DEFAULT 'PENDING',
  attempt_count   INT          NOT NULL DEFAULT 0,
  next_retry_at   DATETIME(3)  NULL,
  degraded        TINYINT      NOT NULL DEFAULT 0,
  last_error      VARCHAR(512) NULL,
  created_at      DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at      DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (order_no),
  KEY idx_status_retry (status, next_retry_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='分账订单级状态';

-- 2) 分账执行记录增列
ALTER TABLE t_split_execution
  ADD COLUMN channel_req_no VARCHAR(64) NULL COMMENT '通道分账幂等单号' AFTER order_no,
  ADD COLUMN degraded      TINYINT     NOT NULL DEFAULT 0 COMMENT '降级模式(仅本地入账)' AFTER channel_split_no,
  ADD KEY idx_channel_req (channel_req_no);

-- 3) 分账审计日志
CREATE TABLE t_split_audit (
  id            BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  biz_type      VARCHAR(32)     NOT NULL COMMENT 'BILL/EXECUTION',
  biz_id        VARCHAR(64)     NOT NULL,
  action        VARCHAR(32)     NOT NULL COMMENT 'APPROVE/REJECT/RETRY',
  operator_type VARCHAR(16)     NOT NULL COMMENT 'MERCHANT/ADMIN/SYSTEM',
  operator_id   BIGINT UNSIGNED NOT NULL DEFAULT 0,
  detail        JSON            NULL,
  created_at    DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  KEY idx_biz (biz_type, biz_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='分账审计日志';