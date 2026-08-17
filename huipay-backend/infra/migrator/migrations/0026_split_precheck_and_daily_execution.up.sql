-- 0026_split_precheck_and_daily_execution.up.sql
-- 分账前置对账 + 每日执行记录迭代（V2 合并版）
-- 说明：
--   - t_split_audit 已在 0022 建，本次补 idx_biz_time 与 idx_action_time_status
--   - t_reconcile_diff 已在 0009 建（含 biz_date），本次补 idx_diff_type_biz_date + merchant_id 列

-- 1) t_store_daily_stats 增加分账状态字段（异步汇总写，热路径不写）
ALTER TABLE t_store_daily_stats
  ADD COLUMN split_status         VARCHAR(16) NOT NULL DEFAULT 'PENDING'
      COMMENT 'PENDING/SUCCESS/PARTIAL/FAILED，由后台任务根据 t_order.split_batch_no 汇总'
      AFTER status_breakdown,
  ADD COLUMN split_batch_no       VARCHAR(64) NULL
      COMMENT '最近一次成功分账的批次号(冗余便于展示)' AFTER split_status,
  ADD COLUMN split_at             DATETIME(3) NULL
      COMMENT '最近一次分账完成时间(SUCCESS/PARTIAL 时)' AFTER split_batch_no,
  ADD COLUMN split_total_amount   BIGINT NOT NULL DEFAULT 0
      COMMENT '该门店×日已被分账的订单合计金额(分)' AFTER split_at,
  ADD KEY idx_split_status (merchant_id, split_status, biz_date);

-- 2) 每日分账执行轨迹表（与 t_split_bill 职责分离）
CREATE TABLE IF NOT EXISTS t_split_daily_execution (
  id                BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  run_id            VARCHAR(64) NOT NULL COMMENT '幂等键 SP_RUN-{merchant}-{batch_no}-{ts}',
  merchant_id       BIGINT UNSIGNED NOT NULL,
  biz_date          DATE NOT NULL,
  batch_no          VARCHAR(64) NOT NULL,
  status            VARCHAR(16) NOT NULL DEFAULT 'RUNNING'
      COMMENT 'RUNNING/SUCCESS/PARTIAL/FAILED',
  started_at        DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  finished_at       DATETIME(3) NULL,
  duration_ms       INT NULL,
  error_code        VARCHAR(64) NULL,
  error_message     VARCHAR(1024) NULL COMMENT '截断+脱敏',
  reconcile_diff_id BIGINT UNSIGNED NULL COMMENT '失败时关联 t_reconcile_diff.id',
  operator_type     VARCHAR(16) NOT NULL DEFAULT 'SYSTEM',
  operator_id       BIGINT UNSIGNED NOT NULL DEFAULT 0,
  PRIMARY KEY (id),
  UNIQUE KEY uk_run_id (run_id),
  KEY idx_merchant_started (merchant_id, started_at),
  KEY idx_status (status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='每日分账执行轨迹';

-- 3) 账单-业务日期关联表（替代 JSON 过滤）
CREATE TABLE IF NOT EXISTS t_split_bill_biz_date (
  bill_id  BIGINT UNSIGNED NOT NULL,
  biz_date DATE NOT NULL,
  PRIMARY KEY (bill_id, biz_date),
  KEY idx_biz_date (biz_date)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='账单-业务日期关联表';

-- 4) t_split_bill 增加冗余 JSON（保留向后兼容，不再用于过滤）
ALTER TABLE t_split_bill
  ADD COLUMN biz_dates JSON NULL
      COMMENT '冗余：账单覆盖业务日期列表(由 t_split_bill_biz_date 派生)'
      AFTER order_nos;

-- 5) t_reconcile_diff 扩展（已存在，补列与索引）
ALTER TABLE t_reconcile_diff
  ADD COLUMN merchant_id BIGINT UNSIGNED NULL COMMENT '所属商户（便于管理端按商户过滤）' AFTER biz_date,
  ADD KEY idx_diff_type_biz_date (diff_type, biz_date),
  ADD KEY idx_merchant_biz_date (merchant_id, biz_date);

-- 6) t_split_audit 补索引（表已在 0022 建）
ALTER TABLE t_split_audit
  ADD KEY idx_biz_time (biz_type, created_at),
  ADD KEY idx_action_time_status (action, created_at);