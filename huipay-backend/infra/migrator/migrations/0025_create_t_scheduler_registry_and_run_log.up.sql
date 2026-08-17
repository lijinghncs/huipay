-- 0025_create_t_scheduler_registry_and_run_log.up.sql
-- 定时任务监测基础设施：注册表 + 运行日志

-- 调度注册表：启动时记录进程内已注册的调度任务（name 唯一）
CREATE TABLE t_scheduler_registry (
  id            BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  name          VARCHAR(64)     NOT NULL COMMENT '调度任务唯一名(store_daily_stats/reconcile_daily/...)',
  display_name  VARCHAR(128)    NOT NULL COMMENT '中文名',
  description   VARCHAR(512)    NULL,
  cron_expr     VARCHAR(64)     NULL COMMENT '人类可读描述(每天02:00)',
  interval_sec  INT             NULL COMMENT '周期(秒)，轮询型调度使用',
  enabled       TINYINT         NOT NULL DEFAULT 1,
  instance_id   VARCHAR(64)     NOT NULL COMMENT '本次进程实例ID(HOSTNAME+PID+启动时间)',
  registered_at DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  UNIQUE KEY uk_name (name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='定时任务注册表';

-- 运行日志：每次执行一条
CREATE TABLE t_scheduler_run_log (
  id            BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  name          VARCHAR(64)     NOT NULL COMMENT '调度任务名',
  instance_id   VARCHAR(64)     NOT NULL,
  biz_date      DATE            NULL     COMMENT '业务日期(报表/对账类)',
  run_mode      VARCHAR(16)     NOT NULL COMMENT 'AUTO/MANUAL',
  status        VARCHAR(16)     NOT NULL COMMENT 'RUNNING/SUCCESS/FAILED/TIMEOUT',
  started_at    DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  finished_at   DATETIME(3)     NULL,
  duration_ms   INT             NULL,
  rows_affected BIGINT          NULL     COMMENT '影响行数(统计/对账)',
  error_message TEXT            NULL,
  trace_id      VARCHAR(64)     NULL,
  PRIMARY KEY (id),
  KEY idx_name_started (name, started_at),
  KEY idx_status_started (status, started_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='定时任务运行日志';