package wechat

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/huipay/huipay-backend/infra/config"
	"github.com/huipay/huipay-backend/internal/domain/vo"
)

// mockProvider 可编程的 WechatConfigProvider，记录调用次数。
type mockProvider struct {
	mu    sync.Mutex
	cfgs  map[uint64]*config.WeChatConfig
	errs  map[uint64]error
	calls map[uint64]int
}

func newMockProvider() *mockProvider {
	return &mockProvider{cfgs: map[uint64]*config.WeChatConfig{}, errs: map[uint64]error{}, calls: map[uint64]int{}}
}

func (p *mockProvider) set(merchantID uint64, cfg *config.WeChatConfig) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cfgs[merchantID] = cfg
}

func (p *mockProvider) setErr(merchantID uint64, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.errs[merchantID] = err
}

func (p *mockProvider) callCount(merchantID uint64) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls[merchantID]
}

func (p *mockProvider) GetRuntimeConfig(ctx context.Context, merchantID uint64) (*config.WeChatConfig, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls[merchantID]++
	if err := p.errs[merchantID]; err != nil {
		return nil, err
	}
	return p.cfgs[merchantID], nil
}

// merchantTestCfg 构造一个可被 wechat.New 解析的商户配置。
func merchantTestCfg(t *testing.T) *config.WeChatConfig {
	t.Helper()
	privPEM, pubPEM := genKeyPEM(t)
	return &config.WeChatConfig{
		Enabled:            true,
		MchID:              "mch_merchant_1",
		AppID:              "app_merchant_1",
		APIv3Key:           "0123456789abcdef0123456789abcdef",
		MerchantSerialNo:   "serial_merchant_1",
		MerchantPrivateKey: privPEM,
		PlatformSerialNo:   "serial_plat_1",
		PlatformPublicKey:  pubPEM,
		NotifyBaseURL:      "https://checkout.huipay.cn",
	}
}

func newTestManager(t *testing.T, provider WechatConfigProvider) (*Manager, *Adapter) {
	t.Helper()
	platform, _ := newTestAdapter(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	return NewManager(platform, provider, zap.NewNop()), platform
}

func TestManagerUnconfiguredMerchantFallsBackToPlatform(t *testing.T) {
	mgr, platform := newTestManager(t, newMockProvider())
	got, err := mgr.Get(context.Background(), 123)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if a, ok := got.(*Adapter); !ok || a != platform {
		t.Fatalf("unconfigured merchant should fallback to platform adapter, got %#v", got)
	}
}

func TestManagerConfiguredMerchant(t *testing.T) {
	provider := newMockProvider()
	cfg := merchantTestCfg(t)
	provider.set(100, cfg)
	mgr, platform := newTestManager(t, provider)

	got, err := mgr.Get(context.Background(), 100)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	a, ok := got.(*Adapter)
	if !ok {
		t.Fatalf("expected *Adapter, got %T", got)
	}
	if a == platform {
		t.Fatal("configured merchant should NOT fallback to platform")
	}
	if a.MerchantID() != 100 {
		t.Fatalf("merchant id = %d, want 100", a.MerchantID())
	}
	if want := "/v1/notify/wechat/100"; a.NotifyPath() != want {
		t.Fatalf("notify path = %q, want %q", a.NotifyPath(), want)
	}
	if a.Config().MchID != "mch_merchant_1" {
		t.Fatalf("mchid = %q, want merchant config", a.Config().MchID)
	}

	// 缓存命中：第二次不重新构造
	got2, err := mgr.Get(context.Background(), 100)
	if err != nil {
		t.Fatalf("get again: %v", err)
	}
	if got2.(*Adapter) != a {
		t.Fatal("second get should hit cache")
	}
	if provider.callCount(100) != 1 {
		t.Fatalf("provider calls = %d, want 1 (cached)", provider.callCount(100))
	}
}

func TestManagerProviderErrorFallsBack(t *testing.T) {
	provider := newMockProvider()
	provider.setErr(7, errors.New("db down"))
	mgr, platform := newTestManager(t, provider)

	got, err := mgr.Get(context.Background(), 7)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if a, ok := got.(*Adapter); !ok || a != platform {
		t.Fatalf("provider error should fallback to platform, got %#v", got)
	}
}

func TestManagerBuildFailFallsBack(t *testing.T) {
	provider := newMockProvider()
	bad := merchantTestCfg(t)
	bad.MerchantPrivateKey = "not-a-valid-pem"
	provider.set(8, bad)
	mgr, platform := newTestManager(t, provider)

	got, err := mgr.Get(context.Background(), 8)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if a, ok := got.(*Adapter); !ok || a != platform {
		t.Fatalf("build failure should fallback to platform, got %#v", got)
	}
}

func TestManagerDisabledMerchantFallsBack(t *testing.T) {
	provider := newMockProvider()
	cfg := merchantTestCfg(t)
	cfg.Enabled = false
	provider.set(9, cfg)
	mgr, platform := newTestManager(t, provider)

	got, err := mgr.Get(context.Background(), 9)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if a, ok := got.(*Adapter); !ok || a != platform {
		t.Fatalf("disabled merchant should fallback to platform, got %#v", got)
	}
}

func TestManagerTTLRefetch(t *testing.T) {
	provider := newMockProvider()
	provider.set(101, merchantTestCfg(t))
	mgr, _ := newTestManager(t, provider)
	mgr.ttl = 20 * time.Millisecond

	if _, err := mgr.Get(context.Background(), 101); err != nil {
		t.Fatalf("get: %v", err)
	}
	time.Sleep(40 * time.Millisecond)
	if _, err := mgr.Get(context.Background(), 101); err != nil {
		t.Fatalf("get after ttl: %v", err)
	}
	if provider.callCount(101) != 2 {
		t.Fatalf("provider calls = %d, want 2 (ttl refetch)", provider.callCount(101))
	}
}

func TestManagerInvalidate(t *testing.T) {
	provider := newMockProvider()
	provider.set(102, merchantTestCfg(t))
	mgr, _ := newTestManager(t, provider)

	if _, err := mgr.Get(context.Background(), 102); err != nil {
		t.Fatalf("get: %v", err)
	}
	mgr.Invalidate(102)
	if _, err := mgr.Get(context.Background(), 102); err != nil {
		t.Fatalf("get after invalidate: %v", err)
	}
	if provider.callCount(102) != 2 {
		t.Fatalf("provider calls = %d, want 2 (invalidated)", provider.callCount(102))
	}
}

func TestManagerGetAdapterNonWechatReturnsNil(t *testing.T) {
	mgr, _ := newTestManager(t, newMockProvider())
	got, err := mgr.GetAdapter(context.Background(), 100, vo.ChannelAlipay)
	if err != nil {
		t.Fatalf("get adapter: %v", err)
	}
	if got != nil {
		t.Fatalf("non-wechat channel should return nil, got %#v", got)
	}
}
