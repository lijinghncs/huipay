-- 0012_add_t_order_code_id.up.sql
-- 订单表增加码牌来源 column：记录订单从哪块收款码牌进入
ALTER TABLE t_order
  ADD COLUMN code_id VARCHAR(16) NULL COMMENT '来源收款码牌短码' AFTER merchant_id;

CREATE INDEX idx_order_code ON t_order (code_id, status, created_at);