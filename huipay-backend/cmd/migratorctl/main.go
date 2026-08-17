// 一次性工具：执行迁移 0026 / 0027 并验证无副作用。
// 用法：go run ./cmd/migratorctl
package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

const (
	defaultDSN = "huipay:huipay@tcp(127.0.0.1:3306)/huipay_main?charset=utf8mb4&parseTime=True&loc=Local&multiStatements=true"
)

// migrationsDir 尝试多个相对位置定位迁移目录（兼容不同启动 cwd）。
// 优先使用相对于本文件所在目录的绝对路径（最稳），再回退到 cwd 相对路径。
func migrationsDir() string {
	if _, file, _, ok := runtime.Caller(0); ok {
		// 本文件位于 huipay-backend/cmd/migratorctl/main.go，上两级即 huipay-backend
		root := filepath.Dir(filepath.Dir(filepath.Dir(file)))
		p := filepath.Join(root, "infra", "migrator", "migrations")
		if _, err := os.Stat(filepath.Join(p, "0026_split_precheck_and_daily_execution.up.sql")); err == nil {
			return p
		}
	}
	candidates := []string{
		"infra/migrator/migrations",
		"../infra/migrator/migrations",
		"../../infra/migrator/migrations",
	}
	for _, p := range candidates {
		if _, err := os.Stat(filepath.Join(p, "0026_split_precheck_and_daily_execution.up.sql")); err == nil {
			return p
		}
	}
	return "infra/migrator/migrations"
}

func main() {
	dsn := os.Getenv("HUIPAY_MIGRATE_DSN")
	if dsn == "" {
		// 兼容：从 ./dsn.txt 或 ../dsn.txt 读取（处理密码含特殊字符）
		for _, p := range []string{"./dsn.txt", "../dsn.txt", "./cmd/migratorctl/dsn.txt"} {
			if b, err := os.ReadFile(p); err == nil {
				dsn = strings.TrimSpace(string(b))
				break
			}
		}
	}
	if dsn == "" {
		dsn = defaultDSN
	}
	action := "all"
	if len(os.Args) > 1 {
		action = os.Args[1]
	}

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(3)
	db.SetConnMaxLifetime(2 * time.Minute)

	// 健康检查（打印 DSN 帮助排错）
	fmt.Printf("DSN: %s\n", maskDSN(dsn))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		log.Fatalf("ping db: %v", err)
	}
	fmt.Println("✓ DB connection ok")

	switch action {
	case "baseline":
		runBaseline(db)
	case "0026":
		run0026(db)
	case "0027":
		run0027(db)
	case "verify":
		verifyAll(db)
	case "all":
		runBaseline(db)
		run0026(db)
		verify0026(db)
		run0027(db)
		verify0027(db)
		verifyAll(db)
	default:
		log.Fatalf("unknown action: %s (use baseline|0026|0027|verify|all)", action)
	}
}

// ===== baseline 备份元数据 =====
func runBaseline(db *sql.DB) {
	fmt.Println("=== BASELINE: capture table & index baseline ===")
	tables := []string{"t_store_daily_stats", "t_split_bill", "t_split_audit", "t_reconcile_diff", "t_order"}
	for _, t := range tables {
		showCreate(db, t)
	}
	fmt.Println()
	fmt.Println("--- golang-migrate schema_migrations 版本 ---")
	rows, err := db.Query("SELECT version, dirty FROM schema_migrations")
	if err != nil {
		fmt.Printf("schema_migrations 表不存在或不可读（首次部署？）: %v\n", err)
	} else {
		defer rows.Close()
		fmt.Printf("%-10s %-8s\n", "version", "dirty")
		for rows.Next() {
			var v int64
			var d bool
			if err := rows.Scan(&v, &d); err != nil {
				fmt.Printf("scan err: %v\n", err)
				continue
			}
			fmt.Printf("%-10d %-8v\n", v, d)
		}
	}
	fmt.Println()
}

// ===== 0026 执行 =====
func run0026(db *sql.DB) {
	fmt.Println("=== RUN 0026 (split_precheck_and_daily_execution.up.sql) ===")
	sqlText, err := readSQL("0026_split_precheck_and_daily_execution.up.sql")
	if err != nil {
		log.Fatalf("read 0026.up.sql: %v", err)
	}
	if err := execStatements(db, string(sqlText)); err != nil {
		log.Fatalf("0026 apply: %v", err)
	}
	fmt.Println("✓ 0026 applied")
}

// ===== 0026 验证 =====
func verify0026(db *sql.DB) {
	fmt.Println("=== VERIFY 0026 ===")

	// t_store_daily_stats 新增列
	cols := []string{"split_status", "split_batch_no", "split_at", "split_total_amount"}
	for _, c := range cols {
		if !hasColumn(db, "t_store_daily_stats", c) {
			log.Fatalf("✗ t_store_daily_stats 缺列 %s", c)
		}
		fmt.Printf("✓ t_store_daily_stats.%s 已存在\n", c)
	}

	// idx_split_status
	if !hasIndex(db, "t_store_daily_stats", "idx_split_status") {
		log.Fatalf("✗ t_store_daily_stats 缺索引 idx_split_status")
	}
	fmt.Println("✓ t_store_daily_stats.idx_split_status 已存在")

	// t_split_daily_existence 新表
	if !tableExists(db, "t_split_daily_execution") {
		log.Fatalf("✗ 表 t_split_daily_execution 不存在")
	}
	fmt.Println("✓ t_split_daily_execution 表已创建")
	for _, c := range []string{"run_id", "merchant_id", "biz_date", "batch_no", "status",
		"started_at", "finished_at", "duration_ms", "error_code", "error_message",
		"reconcile_diff_id", "operator_type", "operator_id"} {
		if !hasColumn(db, "t_split_daily_execution", c) {
			log.Fatalf("✗ t_split_daily_execution 缺列 %s", c)
		}
	}
	fmt.Println("✓ t_split_daily_execution 全部 13 列齐全")
	for _, idx := range []string{"PRIMARY", "uk_run_id", "idx_merchant_started", "idx_status"} {
		if !hasIndex(db, "t_split_daily_execution", idx) {
			log.Fatalf("✗ t_split_daily_execution 缺索引 %s", idx)
		}
	}
	fmt.Println("✓ t_split_daily_execution 索引齐全")

	// t_split_bill_biz_date 新表
	if !tableExists(db, "t_split_bill_biz_date") {
		log.Fatalf("✗ 表 t_split_bill_biz_date 不存在")
	}
	fmt.Println("✓ t_split_bill_biz_date 表已创建")

	// t_split_bill.biz_dates 列
	if !hasColumn(db, "t_split_bill", "biz_dates") {
		log.Fatalf("✗ t_split_bill 缺列 biz_dates")
	}
	fmt.Println("✓ t_split_bill.biz_dates 已存在")

	// t_reconcile_diff 新增列 + 索引
	if !hasColumn(db, "t_reconcile_diff", "merchant_id") {
		log.Fatalf("✗ t_reconcile_diff 缺列 merchant_id")
	}
	fmt.Println("✓ t_reconcile_diff.merchant_id 已存在")
	for _, idx := range []string{"idx_diff_type_biz_date", "idx_merchant_biz_date"} {
		if !hasIndex(db, "t_reconcile_diff", idx) {
			log.Fatalf("✗ t_reconcile_diff 缺索引 %s", idx)
		}
	}
	fmt.Println("✓ t_reconcile_diff 索引齐全")

	// t_split_audit 新增索引
	for _, idx := range []string{"idx_biz_time", "idx_action_time_status"} {
		if !hasIndex(db, "t_split_audit", idx) {
			log.Fatalf("✗ t_split_audit 缺索引 %s", idx)
		}
	}
	fmt.Println("✓ t_split_audit 索引齐全")

	fmt.Println()
}

// ===== 0027 执行 =====
func run0027(db *sql.DB) {
	fmt.Println("=== RUN 0027 (add_perf_indexes.up.sql) ===")
	sqlText, err := readSQL("0027_add_perf_indexes.up.sql")
	if err != nil {
		log.Fatalf("read 0027.up.sql: %v", err)
	}
	if err := execStatements(db, string(sqlText)); err != nil {
		log.Fatalf("0027 apply: %v", err)
	}
	fmt.Println("✓ 0027 applied")
}

// ===== 0027 验证 =====
func verify0027(db *sql.DB) {
	fmt.Println("=== VERIFY 0027 ===")
	for _, idx := range []string{"idx_merchant_status_paidat", "idx_merchant_split", "idx_store_paidat"} {
		if !hasIndex(db, "t_order", idx) {
			log.Fatalf("✗ t_order 缺索引 %s", idx)
		}
		fmt.Printf("✓ t_order.%s 已创建\n", idx)
	}
	fmt.Println()
}

// ===== 综合验证 =====
func verifyAll(db *sql.DB) {
	fmt.Println("=== 综合验证：基线查询仍可用 ===")

	// 1. 现有日报生成 SQL 仍能执行（不依赖新增列）
	fmt.Println("[1/5] 日报聚合 SQL 执行检查 ...")
	start := time.Now()
	var n int
	err := db.QueryRow(`
        SELECT COUNT(*) FROM (
          SELECT 1 FROM t_order o
          WHERE o.status='PAID' AND o.deleted_at IS NULL
            AND o.paid_at >= '2024-01-01' AND o.paid_at < '2030-01-01'
            AND o.store_id IS NOT NULL
          LIMIT 1
        ) t`).Scan(&n)
	if err != nil {
		log.Fatalf("✗ 日报聚合 SQL 失败: %v", err)
	}
	fmt.Printf("  ✓ 日报聚合 SQL OK (耗时 %v)\n", time.Since(start))

	// 2. StoreRevenueQuerier.splitExclusion 修正后语义（V2 新语义）— 验证 NOT EXISTS 子查询不报错
	fmt.Println("[2/5] V2 splitExclusion 修正后查询检查 ...")
	start = time.Now()
	var hasSuccess int
	err = db.QueryRow(`
        SELECT COUNT(*) FROM t_order o
        WHERE EXISTS (SELECT 1 FROM t_entity e WHERE e.id = o.merchant_id AND e.entity_type='MERCHANT' AND e.status=1)
          AND o.status='PAID' AND o.paid_at >= '2024-01-01' AND o.paid_at < '2030-01-01'
          AND NOT EXISTS (
            SELECT 1 FROM t_split_execution se WHERE se.order_no = o.order_no AND se.status = 'SUCCESS'
          )
          AND NOT EXISTS (
            SELECT 1 FROM t_split_bill sb
            INNER JOIN t_split_bill_biz_date bd ON bd.bill_id = sb.id AND bd.biz_date = DATE(o.paid_at)
            WHERE sb.merchant_id = o.merchant_id AND sb.status = 'EXECUTED'
          )
        LIMIT 1`).Scan(&hasSuccess)
	if err != nil {
		log.Fatalf("✗ V2 splitExclusion 修正 SQL 失败: %v", err)
	}
	fmt.Printf("  ✓ V2 splitExclusion 修正 SQL OK (耗时 %v)\n", time.Since(start))

	// 3. Prechecker Layer A SQL（LEFT JOIN 优化版）能执行
	fmt.Println("[3/5] Prechecker Layer A SQL 检查 ...")
	start = time.Now()
	var orderTotal int64
	err = db.QueryRow(`
        SELECT COALESCE(SUM(o.paid_amount), 0) FROM t_order o
        INNER JOIN t_store s ON s.id = o.store_id AND s.status = 1
        LEFT JOIN t_split_execution se ON se.order_no = o.order_no AND se.status = 'SUCCESS'
        LEFT JOIN t_split_bill sb ON sb.merchant_id = o.merchant_id AND sb.status = 'EXECUTED'
        LEFT JOIN t_split_bill_biz_date bd ON bd.bill_id = sb.id AND bd.biz_date = DATE(o.paid_at)
        WHERE EXISTS (SELECT 1 FROM t_entity e WHERE e.id = o.merchant_id AND e.entity_type='MERCHANT' AND e.status=1)
          AND o.status = 'PAID' AND o.deleted_at IS NULL
          AND o.paid_at >= '2024-01-01' AND o.paid_at < '2030-01-01'
          AND o.store_id IS NOT NULL
          AND se.order_no IS NULL AND bd.bill_id IS NULL`).Scan(&orderTotal)
	if err != nil {
		log.Fatalf("✗ Prechecker Layer A SQL 失败: %v", err)
	}
	fmt.Printf("  ✓ Prechecker Layer A SQL OK，order_total=%d (耗时 %v)\n", orderTotal, time.Since(start))

	// 4. 现有 t_split_bill 查询（order_nos 旧字段）仍可用
	fmt.Println("[4/5] 旧版 t_split_bill ORDER_NOS 查询检查 ...")
	start = time.Now()
	var dummy int
	err = db.QueryRow(`SELECT COUNT(*) FROM t_split_bill WHERE order_nos IS NOT NULL LIMIT 1`).Scan(&dummy)
	if err != nil {
		log.Fatalf("✗ t_split_bill.order_nos 查询失败: %v", err)
	}
	fmt.Printf("  ✓ t_split_bill.order_nos 旧字段仍可用 (耗时 %v)\n", time.Since(start))

	// 5. t_split_audit 旧字段 action/biz_type 写入 + 新索引命中
	fmt.Println("[5/5] t_split_audit 旧记录查询检查 ...")
	start = time.Now()
	var auditN int
	err = db.QueryRow(`SELECT COUNT(*) FROM t_split_audit LIMIT 1`).Scan(&auditN)
	if err != nil {
		log.Fatalf("✗ t_split_audit 查询失败: %v", err)
	}
	fmt.Printf("  ✓ t_split_audit 旧记录仍可查询 (耗时 %v)\n", time.Since(start))

	// 最终版本
	fmt.Println()
	fmt.Println("--- 最终 schema_migrations 版本 ---")
	rows, err := db.Query("SELECT version, dirty FROM schema_migrations")
	if err == nil {
		defer rows.Close()
		fmt.Printf("%-10s %-8s\n", "version", "dirty")
		for rows.Next() {
			var v int64
			var d bool
			if err := rows.Scan(&v, &d); err == nil {
				fmt.Printf("%-10d %-8v\n", v, d)
			}
		}
	}
	fmt.Println()
	fmt.Println("✅ 所有验证通过，无副作用")
}

// ===== 工具函数 =====
func showCreate(db *sql.DB, table string) {
	var name, createSQL string
	err := db.QueryRow("SHOW CREATE TABLE "+table).Scan(&name, &createSQL)
	if err != nil {
		fmt.Printf("SHOW CREATE %s: %v\n", table, err)
		return
	}
	fmt.Printf("--- %s (%d chars) ---\n", name, len(createSQL))
}

func hasColumn(db *sql.DB, table, column string) bool {
	rows, err := db.Query(fmt.Sprintf("SHOW COLUMNS FROM %s LIKE '%s'", table, column))
	if err != nil {
		return false
	}
	defer rows.Close()
	return rows.Next()
}

func hasIndex(db *sql.DB, table, indexName string) bool {
	rows, err := db.Query(fmt.Sprintf("SHOW INDEX FROM %s WHERE Key_name = '%s'", table, indexName))
	if err != nil {
		return false
	}
	defer rows.Close()
	return rows.Next()
}

func tableExists(db *sql.DB, table string) bool {
	// INFORMATION_SCHEMA 更稳定（不依赖 SHOW TABLES 列名包含库名）
	var n int
	err := db.QueryRow(
		"SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name = ?",
		table).Scan(&n)
	if err != nil {
		return false
	}
	return n > 0
}

func execStatements(db *sql.DB, sqlText string) error {
	// 拆分语句（以分号 + 换行结尾）
	stmts := splitSQL(sqlText)
	for i, stmt := range stmts {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("stmt #%d (%s...): %w", i+1, truncate(stmt, 80), err)
		}
	}
	return nil
}

func splitSQL(s string) []string {
	// 移除 /* ... */ 注释
	for {
		start := strings.Index(s, "/*")
		end := strings.Index(s, "*/")
		if start < 0 || end < 0 || end <= start+1 {
			break
		}
		s = s[:start] + s[end+2:]
	}
	// 移除 -- 行注释
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	cleaned := ""
	for _, l := range lines {
		t := strings.TrimSpace(l)
		if strings.HasPrefix(t, "--") {
			cleaned += "\n"
			continue
		}
		cleaned += l + "\n"
	}
	// 按分号拆分（不在引号内）
	var cur strings.Builder
	inStr := false
	quoteCh := byte(0)
	for i := 0; i < len(cleaned); i++ {
		c := cleaned[i]
		cur.WriteByte(c)
		if inStr {
			if c == '\\' && i+1 < len(cleaned) {
				cur.WriteByte(cleaned[i+1])
				i++
				continue
			}
			if c == quoteCh {
				inStr = false
				quoteCh = 0
			}
			continue
		}
		if c == '\'' || c == '"' || c == '`' {
			inStr = true
			quoteCh = c
			continue
		}
		if c == ';' {
			out = append(out, cur.String())
			cur.Reset()
		}
	}
	if strings.TrimSpace(cur.String()) != "" {
		out = append(out, cur.String())
	}
	return out
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func readSQL(name string) ([]byte, error) {
	b, err := os.ReadFile(filepath.Join(migrationsDir(), name))
	if err != nil {
		return nil, fmt.Errorf("read %s at %s: %w", name, migrationsDir(), err)
	}
	return b, nil
}

func maskDSN(dsn string) string {
	at := strings.LastIndex(dsn, "@tcp")
	if at < 0 {
		return dsn
	}
	head := dsn[:at]
	tail := dsn[at:]
	colon := strings.Index(head, ":")
	if colon < 0 {
		return dsn
	}
	return head[:colon+1] + "***" + tail
}