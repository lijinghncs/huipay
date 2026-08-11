// 包 entity 定义领域实体。
package entity

// 敏感字段：需 AES 加密存储、只写不回显明文。
var SensitiveFields = []string{"app_secret", "api_v3_key", "merchant_private_key", "platform_public_key"}

// WechatConfig 商户微信支付配置（V3）。
// 敏感字段写库前经 secretcrypto AES-GCM 加密；读出不回显明文。
type WechatConfig struct {
	Enabled            bool   `json:"enabled"`               // 是否启用微信支付
	MchID              string `json:"mchid"`                 // 微信支付商户号
	AppID              string `json:"appid"`                 // 公众号/小程序 AppID
	AppSecret          string `json:"app_secret"`            // 公众号/小程序 AppSecret（敏感）
	APIv3Key           string `json:"api_v3_key"`            // APIv3 密钥（敏感）
	MerchantSerialNo   string `json:"merchant_serial_no"`    // 商户证书序列号
	MerchantPrivateKey string `json:"merchant_private_key"`  // 商户 API 私钥 PEM（敏感）
	PlatformSerialNo   string `json:"platform_serial_no"`    // 平台证书序列号
	PlatformPublicKey  string `json:"platform_public_key"`   // 微信平台公钥 PEM（敏感）
	NotifyBaseURL      string `json:"notify_base_url"`       // 回调地址前缀
}