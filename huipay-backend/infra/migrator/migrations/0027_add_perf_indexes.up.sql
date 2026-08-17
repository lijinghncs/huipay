-- 0027_add_perf_indexes.up.sql
-- 分账前置对账性能索引（V2 合并版）
-- 服务于：
--   - Prechecker 双层对账（商户级 + 门店×日）
--   - 门店日报补跑（Backfill）
--   - async 汇总（RecomputeByMerchantDate）
-- 部署建议：使用 gh-ost 或 pt-online-schema-change 在线 DDL；ALGORITHM=INPLACE 备用方案

-- 1) Prechecker / 聚合查询主索引
ALTER TABLE t_order
  ADD KEY idx_merchant_status_paidat (merchant_id, status, paid_at);

-- 2) split_batch_no IS NOT NULL 过滤辅助（async 汇总用）
ALTER TABLE t_order
  ADD KEY idx_merchant_split (merchant_id, split_batch_no, status, paid_at);

-- 3) 单门店按 paid_at 反查（async 汇总 + 单门店回算）
ALTER TABLE t_order
  ADD KEY idx_store_paidat (store_id, status, paid_at);