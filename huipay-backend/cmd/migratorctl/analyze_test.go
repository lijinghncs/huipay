// 一次性 ANALYZE 脚本。
//go:build ignore

package main

import (
	"database/sql"
	"fmt"
	"log"

	_ "github.com/go-sql-driver/mysql"
)

func main() {
	db, err := sql.Open("mysql", "root:lijing123!@#@tcp(127.0.0.1:3306)/huipay?charset=utf8mb4&parseTime=True&loc=Local&multiStatements=true")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	for _, t := range []string{"t_order", "t_store_daily_stats", "t_split_daily_execution", "t_split_bill_biz_date", "t_reconcile_diff", "t_split_audit", "t_split_bill"} {
		if _, err := db.Exec("ANALYZE TABLE " + t); err != nil {
			fmt.Printf("ANALYZE %s: %v\n", t, err)
			continue
		}
		fmt.Printf("ANALYZE %s OK\n", t)
	}
}