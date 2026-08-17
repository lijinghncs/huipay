-- 0024_create_t_store_daily_stats.up.sql
-- 门店订单日报(T+1聚合)：按 门店 × 日 聚合 PAID 订单笔数与金额

CREATE TABLE t_store_daily_stats (
  id              BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  merchant_id     BIGINT UNSIGNED NOT NULL COMMENT '所属商户',
  store_id        BIGINT UNSIGNED NOT NULL COMMENT '门店 ID',
  biz_date        DATE            NOT NULL COMMENT '业务日期 YYYY-MM-DD',
  order_count     INT             NOT NULL DEFAULT 0 COMMENT '订单总笔数(PAID)',
  paid_amount     BIGINT          NOT NULL DEFAULT 0 COMMENT '订单总金额(分,SUM(paid_amount))',
  channel_breakdown JSON          NULL     COMMENT '各渠道拆分 {WECHAT:{count,amount}, ...}',
  status_breakdown  JSON          NULL     COMMENT '各状态笔数 {PAID:n,REFUNDED:n,CLOSED:n}',
  generated_at    DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3) COMMENT '统计生成时间',
  updated_at      DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  UNIQUE KEY uk_store_date (store_id, biz_date),
  KEY idx_merchant_date (merchant_id, biz_date),
  KEY idx_date (biz_date)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='门店订单日报(T+1聚合)';