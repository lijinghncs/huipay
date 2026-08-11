package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/glebarez/sqlite"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/huipay/huipay-backend/internal/domain/entity"
	"github.com/huipay/huipay-backend/internal/merchant/repository"
	"github.com/huipay/huipay-backend/internal/merchant/secretcrypto"
)

var runtimeDBSeq uint64

// buildRuntimeDB 建内存库 + EntityRepo + Service（accountSvc 用 nil，GetRuntimeConfig 不依赖账户服务）。
func buildRuntimeDB(t *testing.T) (*Service, *repository.EntityRepo) {
	t.Helper()
	n := atomic.AddUint64(&runtimeDBSeq, 1)
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:rt%d?mode=memory&cache=shared", n)), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&repository.EntityModel{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repo := repository.NewEntityRepo(db)
	svc := NewService(db, repo, nil, zap.NewNop(), "test-secret")
	return svc, repo
}

// storedConfig 把明文配置加密为存储 JSON（敏感字段 AES 加密）。
func storedConfig(t *testing.T, cfg map[string]any) string {
	t.Helper()
	sensitive := map[string]bool{}
	for _, f := range entity.SensitiveFields {
		sensitive[f] = true
	}
	out := make(map[string]any, len(cfg))
	for k, v := range cfg {
		if s, ok := v.(string); ok && sensitive[k] {
			enc, err := secretcrypto.Encrypt(s)
			if err != nil {
				t.Fatalf("encrypt %s: %v", k, err)
			}
			out[k] = enc
		} else {
			out[k] = v
		}
	}
	b, _ := json.Marshal(out)
	return string(b)
}

func TestGetRuntimeConfigConfigured(t *testing.T) {
	svc, repo := buildRuntimeDB(t)
	m := &repository.EntityModel{
		EntityCode:   "M_TEST_RT_1",
		EntityType:   "MERCHANT",
		Name:         "运行时配置测试",
		KYCStatus:    1,
		Status:       1,
		WechatConfig: storedConfig(t, map[string]any{
			"enabled":              true,
			"mchid":                "mch_rt_1",
			"appid":                "app_rt_1",
			"app_secret":           "secret_rt_1",
			"api_v3_key":           "key_rt_1",
			"merchant_serial_no":   "serial_m_rt_1",
			"merchant_private_key": "priv_rt_1",
			"platform_serial_no":   "serial_p_rt_1",
			"platform_public_key":  "pub_rt_1",
			"notify_base_url":      "https://checkout.huipay.cn",
		}),
	}
	if err := repo.Create(context.Background(), m); err != nil {
		t.Fatalf("create entity: %v", err)
	}

	cfg, err := svc.GetRuntimeConfig(context.Background(), m.ID)
	if err != nil {
		t.Fatalf("get runtime config: %v", err)
	}
	if cfg == nil || !cfg.Enabled {
		t.Fatalf("expected enabled config, got %#v", cfg)
	}
	if cfg.MchID != "mch_rt_1" || cfg.AppID != "app_rt_1" {
		t.Fatalf("mchid/appid = %q/%q", cfg.MchID, cfg.AppID)
	}
	for key, want := range map[string]string{
		"app_secret":           "secret_rt_1",
		"api_v3_key":           "key_rt_1",
		"merchant_private_key": "priv_rt_1",
		"platform_public_key":  "pub_rt_1",
	} {
		got := map[string]string{
			"app_secret":           cfg.AppSecret,
			"api_v3_key":           cfg.APIv3Key,
			"merchant_private_key": cfg.MerchantPrivateKey,
			"platform_public_key":  cfg.PlatformPublicKey,
		}[key]
		if got != want {
			t.Fatalf("%s = %q, want %q (decrypted)", key, got, want)
		}
	}
	if cfg.NotifyBaseURL != "https://checkout.huipay.cn" {
		t.Fatalf("notify_base_url = %q", cfg.NotifyBaseURL)
	}
}

func TestGetRuntimeConfigUnconfigured(t *testing.T) {
	svc, repo := buildRuntimeDB(t)
	m := &repository.EntityModel{EntityCode: "M_TEST_RT_2", EntityType: "MERCHANT", Name: "未配置", Status: 1}
	if err := repo.Create(context.Background(), m); err != nil {
		t.Fatalf("create entity: %v", err)
	}
	cfg, err := svc.GetRuntimeConfig(context.Background(), m.ID)
	if err != nil {
		t.Fatalf("get runtime config: %v", err)
	}
	if cfg != nil {
		t.Fatalf("unconfigured merchant should return nil, got %#v", cfg)
	}
}

func TestGetRuntimeConfigDisabled(t *testing.T) {
	svc, repo := buildRuntimeDB(t)
	m := &repository.EntityModel{
		EntityCode: "M_TEST_RT_3", EntityType: "MERCHANT", Name: "禁用", Status: 1,
		WechatConfig: storedConfig(t, map[string]any{"enabled": false, "mchid": "mch_rt_3"}),
	}
	if err := repo.Create(context.Background(), m); err != nil {
		t.Fatalf("create entity: %v", err)
	}
	cfg, err := svc.GetRuntimeConfig(context.Background(), m.ID)
	if err != nil {
		t.Fatalf("get runtime config: %v", err)
	}
	if cfg != nil {
		t.Fatalf("disabled merchant should return nil, got %#v", cfg)
	}
}

func TestGetRuntimeConfigMissingEntity(t *testing.T) {
	svc, _ := buildRuntimeDB(t)
	cfg, err := svc.GetRuntimeConfig(context.Background(), 999999)
	if err != nil {
		t.Fatalf("get runtime config: %v", err)
	}
	if cfg != nil {
		t.Fatalf("missing merchant should return nil, got %#v", cfg)
	}
}
