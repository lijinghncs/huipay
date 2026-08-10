package wechat

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"strings"
	"testing"
	"time"
)

// genKeyPEM 生成 RSA 密钥对并返回 PKCS#1 私钥 PEM 与 PKIX 公钥 PEM（测试用）。
func genKeyPEM(t *testing.T) (privPEM, pubPEM string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("gen key: %v", err)
	}
	privPEM = string(pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	}))
	pubDER, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatalf("marshal pub: %v", err)
	}
	pubPEM = string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER}))
	return
}

func TestSignVerifyRoundTrip(t *testing.T) {
	privPEM, pubPEM := genKeyPEM(t)
	privKey, err := parsePrivateKey(privPEM)
	if err != nil {
		t.Fatalf("parse private: %v", err)
	}
	pubKey, err := parsePublicKey(pubPEM)
	if err != nil {
		t.Fatalf("parse public: %v", err)
	}

	signStr := buildRequestSignStr("POST", "/v3/pay/transactions/native", "1700000000", "nonce123", `{"a":1}`)
	sig, err := rsaSHA256Sign(privKey, signStr)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	if err := rsaSHA256Verify(pubKey, signStr, sig); err != nil {
		t.Fatalf("verify should pass: %v", err)
	}

	// 篡改签名串后验签应失败
	if err := rsaSHA256Verify(pubKey, signStr+"x", sig); err == nil {
		t.Fatal("verify should fail on tampered message")
	}
}

func TestParsePublicKeyFromCertificate(t *testing.T) {
	privPEM, _ := genKeyPEM(t)
	privKey, err := parsePrivateKey(privPEM)
	if err != nil {
		t.Fatalf("parse private: %v", err)
	}
	// 生成自签证书，验证 CERTIFICATE PEM 也能解析出公钥
	now := time.Now()
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkixName(),
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &privKey.PublicKey, privKey)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}
	certPEM := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	if _, err := parsePublicKey(certPEM); err != nil {
		t.Fatalf("parse cert public: %v", err)
	}
}

// pkixName 构造证书 Subject（避免模板缺省字段导致自签失败）。
func pkixName() pkix.Name {
	return pkix.Name{CommonName: "wechat test"}
}

func TestInvalidPrivateKey(t *testing.T) {
	if _, err := parsePrivateKey("not-a-pem"); err == nil {
		t.Fatal("expected error for invalid private key")
	}
}

func TestBuildAuthHeader(t *testing.T) {
	h := buildAuthHeader("mch123", "serialABC", "1700000000", "nonce123", "sig===")
	for _, want := range []string{
		"WECHATPAY2-SHA256-RSA2048",
		`mchid="mch123"`,
		`serial_no="serialABC"`,
		`timestamp="1700000000"`,
		`nonce_str="nonce123"`,
		`signature="sig==="`,
	} {
		if !strings.Contains(h, want) {
			t.Fatalf("auth header missing %q: %s", want, h)
		}
	}
}

func TestSignStrFormat(t *testing.T) {
	s := buildRequestSignStr("POST", "/v3/pay/transactions/native", "1700000000", "nonce123", `{}`)
	want := "POST\n/v3/pay/transactions/native\n1700000000\nnonce123\n{}\n"
	if s != want {
		t.Fatalf("sign str mismatch:\n got %q\nwant %q", s, want)
	}
}