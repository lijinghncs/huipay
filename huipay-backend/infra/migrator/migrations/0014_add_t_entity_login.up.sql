-- 0014_add_t_entity_login.up.sql
-- 主体表增加商户登录字段：手机号（唯一）+ 登录密码哈希（bcrypt）
ALTER TABLE t_entity
  ADD COLUMN login_phone VARCHAR(32) NULL COMMENT '登录手机号' AFTER wechat_config,
  ADD COLUMN login_password_hash VARCHAR(128) NULL COMMENT '登录密码哈希（bcrypt）' AFTER login_phone;

CREATE UNIQUE INDEX uk_login_phone ON t_entity (login_phone);
