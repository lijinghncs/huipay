package bootstrap

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"

	"go.uber.org/zap"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/huipay/huipay-backend/internal/account/ledger"
	"github.com/huipay/huipay-backend/internal/account/repository"
	accountsvc "github.com/huipay/huipay-backend/internal/account/service"
)

var memDBSeq int64

// newMemDSN 返回唯一的共享内存 SQLite DSN，避免跨测试复用同一数据库。
func newMemDSN() string {
	n := atomic.AddInt64(&memDBSeq, 1)
	return fmt.Sprintf("file:mem%d?mode=memory&cache=shared", n)
}

func newSeedTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(newMemDSN()), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&entity{}, &repository.WalletModel{}, &repository.JournalEntryModel{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func TestSeedChannelSettlementWallets(t *testing.T) {
	db := newSeedTestDB(t)
	ledgerSvc := ledger.NewService(repository.NewWalletRepo(db), repository.NewJournalRepo(db), zap.NewNop())
	accountSvc := accountsvc.NewService(ledgerSvc, repository.NewWalletRepo(db), repository.NewJournalRepo(db), zap.NewNop())

	ctx := context.Background()
	wid1, err := SeedChannelSettlementWallets(ctx, db, accountSvc, zap.NewNop())
	if err != nil {
		t.Fatalf("first seed: %v", err)
	}
	if wid1 == 0 {
		t.Fatal("wechat settlement wallet id should be non-zero")
	}

	// 断言微信 + 支付宝 entity 均存在
	var wechatEntity entity
	if err := db.Where("entity_code = ?", EntityCodeChannelSettlementWeChat).First(&wechatEntity).Error; err != nil {
		t.Fatalf("wechat entity missing: %v", err)
	}
	var alipayEntity entity
	if err := db.Where("entity_code = ?", EntityCodeChannelSettlementAlipay).First(&alipayEntity).Error; err != nil {
		t.Fatalf("alipay entity missing: %v", err)
	}

	// 重复启动 → 幂等，返回相同 wallet_id，不重复创建
	wid2, err := SeedChannelSettlementWallets(ctx, db, accountSvc, zap.NewNop())
	if err != nil {
		t.Fatalf("second seed: %v", err)
	}
	if wid2 != wid1 {
		t.Fatalf("second seed wallet_id = %d, want %d", wid2, wid1)
	}

	var cnt int64
	if err := db.Model(&repository.WalletModel{}).Count(&cnt).Error; err != nil {
		t.Fatalf("count wallets: %v", err)
	}
	if cnt != 2 {
		t.Fatalf("wallets = %d, want 2 (wechat + alipay)", cnt)
	}
}