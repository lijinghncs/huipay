-- 0010_add_idx_merchant_created.up.sql
-- 商家订单列表索引：ListByMerchant 按 (merchant_id, created_at DESC) 分页，
-- 现有 idx_status_created 与 uk_merchant_order 均不支持，新增复合索引避免全表扫。
ALTER TABLE t_order ADD KEY idx_merchant_created (merchant_id, created_at DESC);