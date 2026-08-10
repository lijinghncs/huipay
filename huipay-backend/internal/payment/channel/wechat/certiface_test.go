package wechat

import (
	"context"
	"testing"
)

// TestStaticCertProviderGetBySerial 静态证书按序列号返回公钥。
func TestStaticCertProviderGetBySerial(t *testing.T) {
	_, pubPEM := genKeyPEM(t)
	pub, err := parsePublicKey(pubPEM)
	if err != nil {
		t.Fatalf("parse public key: %v", err)
	}
	p := &StaticCertProvider{key: pub}

	got, err := p.GetBySerial(context.Background(), "serial_plat")
	if err != nil {
		t.Fatalf("GetBySerial: %v", err)
	}
	if got == nil {
		t.Fatal("GetBySerial returned nil key")
	}
}

// TestStaticCertProviderNotConfigured 未配置公钥时返回错误。
func TestStaticCertProviderNotConfigured(t *testing.T) {
	p := &StaticCertProvider{}
	if _, err := p.GetBySerial(context.Background(), "s"); err == nil {
		t.Fatal("expected error when key not configured")
	}
}

// TestStaticCertProviderRefresh 静态实现 Refresh 为空操作，不报错。
func TestStaticCertProviderRefresh(t *testing.T) {
	p := &StaticCertProvider{}
	if err := p.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh: %v", err)
	}
}