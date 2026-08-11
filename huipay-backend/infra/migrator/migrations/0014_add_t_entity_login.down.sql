-- 0014_add_t_entity_login.down.sql
ALTER TABLE t_entity
  DROP INDEX uk_login_phone,
  DROP COLUMN login_password_hash,
  DROP COLUMN login_phone;
