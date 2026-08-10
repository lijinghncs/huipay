-- 0008_add_t_order_closed_at.up.sql
-- 订单关单时间（超时关单定时任务写入）
ALTER TABLE t_order ADD COLUMN closed_at DATETIME(3) NULL COMMENT '关单时间';