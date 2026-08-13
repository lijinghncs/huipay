-- 0019_wallet_entity_type_unique.up.sql
-- 钱包唯一键由 (entity_id, currency) 扩展为 (entity_id, entity_type, currency)，
-- 使门店/商户/通道户等不同类型主体可拥有各自钱包，避免 id 自增空间冲突。
ALTER TABLE t_wallet
  DROP KEY uk_entity_currency,
  ADD UNIQUE KEY uk_entity_type_currency (entity_id, entity_type, currency);