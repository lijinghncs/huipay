-- 0028_add_split_mode_to_t_entity.up.sql
-- 分账模式配置化（C1 层补全）：AUTO 自动降级 / LOCAL_ONLY 仅本地记账 / CHANNEL_REQUIRED 强制通道

ALTER TABLE t_entity
  ADD COLUMN split_mode VARCHAR(16) NOT NULL DEFAULT 'AUTO' COMMENT '分账模式：AUTO/LOCAL_ONLY/CHANNEL_REQUIRED' AFTER wechat_config;
