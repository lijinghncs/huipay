-- 0021_add_order_nos_to_t_split_bill.up.sql
-- 分账单增加覆盖订单号列表(order_nos)，用于分账时排除已分账订单，避免重复分账
ALTER TABLE t_split_bill
  ADD COLUMN order_nos JSON NULL COMMENT '账单覆盖的订单号列表(JSON数组)';