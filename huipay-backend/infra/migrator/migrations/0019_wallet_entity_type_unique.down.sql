-- 0019_wallet_entity_type_unique.down.sql
ALTER TABLE t_wallet
  DROP KEY uk_entity_type_currency,
  ADD UNIQUE KEY uk_entity_currency (entity_id, currency);