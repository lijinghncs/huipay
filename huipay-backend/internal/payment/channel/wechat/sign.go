// 包 wechat 实现微信支付 V3 通道：签名、验签、回调解密与 HTTP 客户端。
// 本文件为签名与验签：商户私钥请求签名（RSA-SHA256）+ 平台证书验签。
package wechat

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
)

// parsePrivateKey 解析商户 API 私钥（支持 PKCS#1 / PKCS#8 PEM）。
func parsePrivateKey(pemStr string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return nil, fmt.Errorf("wechat: invalid merchant private key PEM")
	}
	if key, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		rk, ok := key.(*rsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("wechat: private key is not RSA")
		}
		return rk, nil
	}
	rk, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("wechat: parse private key: %w", err)
	}
	return rk, nil
}

// parsePublicKey 解析平台证书公钥（支持 CERTIFICATE / PKIX / PKCS#1 PEM）。
func parsePublicKey(pemStr string) (*rsa.PublicKey, error) {
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return nil, fmt.Errorf("wechat: invalid platform public key PEM")
	}
	if block.Type == "CERTIFICATE" {
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("wechat: parse certificate: %w", err)
		}
		rk, ok := cert.PublicKey.(*rsa.PublicKey)
		if !ok {
			return nil, fmt.Errorf("wechat: certificate public key is not RSA")
		}
		return rk, nil
	}
	if key, err := x509.ParsePKIXPublicKey(block.Bytes); err == nil {
		rk, ok := key.(*rsa.PublicKey)
		if !ok {
			return nil, fmt.Errorf("wechat: public key is not RSA")
		}
		return rk, nil
	}
	rk, err := x509.ParsePKCS1PublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("wechat: parse public key: %w", err)
	}
	return rk, nil
}

// buildRequestSignStr 构造请求签名串（微信 V3 规范）。
// canonicalURL 为规范化 URL（含 query，如 /v3/pay/transactions/native）。
func buildRequestSignStr(method, canonicalURL, timestamp, nonce, body string) string {
	return method + "\n" + canonicalURL + "\n" + timestamp + "\n" + nonce + "\n" + body + "\n"
}

// buildVerifySignStr 构造验签签名串（回调 / 响应验签使用）。
func buildVerifySignStr(timestamp, nonce, body string) string {
	return timestamp + "\n" + nonce + "\n" + body + "\n"
}

// rsaSHA256Sign 使用商户私钥对签名串做 RSA-SHA256 签名，返回 base64。
func rsaSHA256Sign(privateKey *rsa.PrivateKey, signStr string) (string, error) {
	digest := sha256.Sum256([]byte(signStr))
	sig, err := rsa.SignPKCS1v15(rand.Reader, privateKey, crypto.SHA256, digest[:])
	if err != nil {
		return "", fmt.Errorf("wechat: sign fail: %w", err)
	}
	return base64.StdEncoding.EncodeToString(sig), nil
}

// rsaSHA256Verify 使用平台证书公钥验签。
func rsaSHA256Verify(publicKey *rsa.PublicKey, signStr, signature string) error {
	digest := sha256.Sum256([]byte(signStr))
	sig, err := base64.StdEncoding.DecodeString(signature)
	if err != nil {
		return fmt.Errorf("wechat: decode signature: %w", err)
	}
	if err := rsa.VerifyPKCS1v15(publicKey, crypto.SHA256, digest[:], sig); err != nil {
		return fmt.Errorf("wechat: verify signature fail: %w", err)
	}
	return nil
}

// buildAuthHeader 构造 V3 请求的 Authorization 头。
func buildAuthHeader(mchID, serialNo, timestamp, nonce, signature string) string {
	return fmt.Sprintf(
		`WECHATPAY2-SHA256-RSA2048 mchid="%s",nonce_str="%s",signature="%s",timestamp="%s",serial_no="%s"`,
		mchID, nonce, signature, timestamp, serialNo,
	)
}