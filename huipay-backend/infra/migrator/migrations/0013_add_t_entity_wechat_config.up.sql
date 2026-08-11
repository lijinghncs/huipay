-- 0013_add_t_entity_wechat_config.up.sql
-- 主体表增加商户微信支付配置列：整体为 JSON，敏感字段以 AES 密文存储
ALTER TABLE t_entity
  ADD COLUMN wechat_config JSON NULL COMMENT '商户微信支付配置（敏感字段已 AES 加密）' AFTER kyc_data;