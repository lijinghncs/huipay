package adapter

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/huipay/huipay-backend/internal/recon/domain"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// TestSmokeMySQL 在真实 MySQL 上冒烟验证 recon 适配器 SQL（HUIPAY_SMOKE_DSN 未设置时跳过）。
func TestSmokeMySQL(t *testing.T) {
	dsn := os.Getenv("HUIPAY_SMOKE_DSN")
	if dsn == "" {
		t.Skip("HUIPAY_SMOKE_DSN not set")
	}
	sqlDB, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatal(err)
	}
	db, err := gorm.Open(mysql.New(mysql.Config{Conn: sqlDB}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	bizDate := time.Now().AddDate(0, 0, -1)

	of := NewOrderFetcher(db)
	sf := NewSplitFetcher(db)

	merchants, err := sf.ListMerchantsWithExecution(ctx, bizDate, bizDate.AddDate(0, 0, 1))
	if err != nil {
		t.Fatalf("ListMerchantsWithExecution: %v", err)
	}
	fmt.Printf("merchants with execution: %v\n", merchants)
	for _, mid := range merchants {
		rows, err := sf.SumByOrder(ctx, mid, bizDate, bizDate.AddDate(0, 0, 1))
		if err != nil {
			t.Fatalf("SumByOrder(%d): %v", mid, err)
		}
		fmt.Printf("merchant %d exec rows: %d\n", mid, len(rows))
		var orderNos []string
		for _, r := range rows {
			orderNos = append(orderNos, r.OrderNo)
		}
		if len(orderNos) > 0 {
			journal, err := sf.SumByOrderNos(ctx, orderNos)
			if err != nil {
				t.Fatalf("SumByOrderNos: %v", err)
			}
			fmt.Printf("merchant %d journal sums: %v\n", mid, journal)
		}
	}

	total, err := of.SumForSplit(ctx, 1, bizDate, bizDate.AddDate(0, 0, 1))
	if err != nil {
		t.Fatalf("SumForSplit: %v", err)
	}
	fmt.Printf("SumForSplit(merchant=1): %d\n", total)

	byStore, err := of.SumByStoreAndDate(ctx, 1, bizDate, bizDate.AddDate(0, 0, 1))
	if err != nil {
		t.Fatalf("SumByStoreAndDate: %v", err)
	}
	fmt.Printf("SumByStoreAndDate rows: %d\n", len(byStore))

	paid, err := of.ListPaidForChannel(ctx, bizDate, bizDate.AddDate(0, 0, 1))
	if err != nil {
		t.Fatalf("ListPaidForChannel: %v", err)
	}
	fmt.Printf("ListPaidForChannel rows: %d\n", len(paid))
	if len(paid) > 0 {
		var orderNos []string
		for _, o := range paid {
			orderNos = append(orderNos, o.OrderNo)
		}
		mm, err := of.MerchantsByOrderNos(ctx, orderNos)
		if err != nil {
			t.Fatalf("MerchantsByOrderNos: %v", err)
		}
		fmt.Printf("MerchantsByOrderNos: %v\n", mm)
	}

	_ = domain.DiffTypeSplitPost
	fmt.Println("SMOKE OK")
}
