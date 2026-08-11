-- 0008_add_t_order_closed_at.up.sql
-- 补充 t_order.closed_at 列（与 OrderModel 对齐，用于记录关单时间）
ALTER TABLE t_order
  ADD COLUMN closed_at DATETIME(3) NULL AFTER paid_at;