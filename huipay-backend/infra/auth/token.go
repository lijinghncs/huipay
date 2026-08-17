// 包 auth 提供商户登录态 token 的签发与校验。
// 实现为 HMAC-SHA256 签名的紧凑 token（header.payload.signature，JWT 风格），仅标准库依赖。
package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Claims token 载荷。
type Claims struct {
	MerchantID uint64 `json:"merchant_id"`
	Role       string `json:"role,omitempty"` // 空=商户；"admin"=管理后台。兼容既有商户 token（无该字段）。
	Exp        int64  `json:"exp"`            // Unix 秒
}

// tokenTTL 登录态有效期。
const tokenTTL = 24 * time.Hour

// Sign 签发商户 token。
func Sign(secret string, merchantID uint64) (string, error) {
	if merchantID == 0 {
		return "", errors.New("auth: merchant id required")
	}
	payload, err := json.Marshal(Claims{MerchantID: merchantID, Exp: time.Now().Add(tokenTTL).Unix()})
	if err != nil {
		return "", err
	}
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	body := base64.RawURLEncoding.EncodeToString(payload)
	sig := hmacSHA256(secret, header+"."+body)
	return header + "." + body + "." + sig, nil
}

// SignAdmin 签发管理后台 token（adminID 作为标识，Role=admin）。
func SignAdmin(secret string, adminID uint64) (string, error) {
	if adminID == 0 {
		return "", errors.New("auth: admin id required")
	}
	payload, err := json.Marshal(Claims{MerchantID: adminID, Role: "admin", Exp: time.Now().Add(tokenTTL).Unix()})
	if err != nil {
		return "", err
	}
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	body := base64.RawURLEncoding.EncodeToString(payload)
	sig := hmacSHA256(secret, header+"."+body)
	return header + "." + body + "." + sig, nil
}

// Verify 校验 token 并返回载荷。
func Verify(secret, token string) (*Claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, errors.New("auth: malformed token")
	}
	expect := hmacSHA256(secret, parts[0]+"."+parts[1])
	if !hmac.Equal([]byte(expect), []byte(parts[2])) {
		return nil, errors.New("auth: invalid signature")
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, errors.New("auth: invalid payload")
	}
	var c Claims
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, fmt.Errorf("auth: parse claims: %w", err)
	}
	if c.MerchantID == 0 {
		return nil, errors.New("auth: missing merchant id")
	}
	if c.Exp < time.Now().Unix() {
		return nil, errors.New("auth: token expired")
	}
	return &c, nil
}

func hmacSHA256(secret, data string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(data))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
