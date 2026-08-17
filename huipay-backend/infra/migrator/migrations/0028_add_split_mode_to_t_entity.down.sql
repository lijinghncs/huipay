-- 0028_add_split_mode_to_t_entity.down.sql
-- 回滚分账模式列

ALTER TABLE t_entity
  DROP COLUMN split_mode;
