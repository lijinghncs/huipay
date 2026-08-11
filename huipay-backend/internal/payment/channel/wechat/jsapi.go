// JSAPI 支付前端调起参数生成。
// 微信支付 V3 中，微信内拉起（getBrandWCPayRequest）需要后台对 prepay_id 做二次签名：
// 签名串为 appId\n timeStamp\n nonceStr\n package\n signType\n，用 APIv3Key 做 HMAC-SHA256。
package wechat

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"time"

	"github.com/huipay/huipay-backend/internal/payment/channel"
)

const jsapiSignType = "HMAC-SHA256"

// BuildJSAPIParams 基于 prepay_id 生成微信内拉起 JSAPI 支付的调起参数。
func BuildJSAPIParams(appID, prepayID, apiV3Key string) (*channel.JSAPIParams, error) {
	if appID == "" || prepayID == "" || apiV3Key == "" {
		return nil, fmt.Errorf("wechat jsapi: appid/prepay_id/api_v3_key required")
	}
	timeStamp := strconv.FormatInt(time.Now().Unix(), 10)
	nonceStr := randomNonce()
	pkg := "prepay_id=" + prepayID

	signStr := appID + "\n" + timeStamp + "\n" + nonceStr + "\n" + pkg + "\n" + jsapiSignType + "\n"
	paySign, err := hmacSHA256Hex(apiV3Key, signStr)
	if err != nil {
		return nil, err
	}

	return &channel.JSAPIParams{
		AppID:     appID,
		TimeStamp: timeStamp,
		NonceStr:  nonceStr,
		Package:   pkg,
		SignType:  jsapiSignType,
		PaySign:   paySign,
	}, nil
}

// hmacSHA256Hex 使用密钥对明文做 HMAC-SHA256，输出小写十六进制。
func hmacSHA256Hex(key, message string) (string, error) {
	mac := hmac.New(sha256.New, []byte(key))
	_, err := mac.Write([]byte(message))
	if err != nil {
		return "", fmt.Errorf("wechat jsapi: hmac write: %w", err)
	}
	return hex.EncodeToString(mac.Sum(nil)), nil
}