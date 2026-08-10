// 包 wechat 平台证书获取接口（当前为静态实现，动态下载见 infra/wechat/cert/design.md）。
package wechat

import (
	"context"
	"crypto/rsa"
	"fmt"
)

// CertProvider 平台证书动态获取接口（未来实装）。
type CertProvider interface {
	GetBySerial(ctx context.Context, serial string) (*rsa.PublicKey, error)
	Refresh(ctx context.Context) error
}

// StaticCertProvider 静态证书（当前实现，注入 PEM 即用）。
type StaticCertProvider struct{ key *rsa.PublicKey }

func (s *StaticCertProvider) GetBySerial(ctx context.Context, serial string) (*rsa.PublicKey, error) {
	if s.key == nil {
		return nil, fmt.Errorf("wechat: platform public key not configured")
	}
	return s.key, nil
}

func (s *StaticCertProvider) Refresh(ctx context.Context) error { return nil }