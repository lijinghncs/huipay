package repository

import "gorm.io/gorm/clause"

// clauseIgnore 返回「重复主键跳过」子句（MySQL INSERT IGNORE）。
func clauseIgnore() clause.Insert {
	return clause.Insert{Modifier: "IGNORE"}
}