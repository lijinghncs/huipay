// 包 config 提供应用配置加载能力，基于 Viper 读取 config.yaml 与 ENV。
package config

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

// Config 应用全局配置。
type Config struct {
	HTTPPort    int    `mapstructure:"http_port"`
	GinMode     string `mapstructure:"gin_mode"`     // debug / release / test
	LogLevel    string `mapstructure:"log_level"`    // debug / info / warn / error
	MySQLMaster string `mapstructure:"mysql_master"` // 主库 DSN
	MySQLSlave  string `mapstructure:"mysql_slave"`  // 从库 DSN（可空）
	AppName     string `mapstructure:"app_name"`
	AppEnv      string `mapstructure:"app_env"` // local / staging / production
	WeChat      WeChatConfig `mapstructure:"wechat"`
}

// WeChatConfig 微信支付 V3 相关配置。
// 密钥类字段建议通过环境变量注入（HUIPAY_WECHAT_*），避免明文入库。
type WeChatConfig struct {
	Enabled            bool   `mapstructure:"enabled"`               // 是否启用微信支付通道
	MchID              string `mapstructure:"mchid"`                 // 微信支付商户号
	AppID              string `mapstructure:"appid"`                 // 公众号/小程序 AppID
	AppSecret          string `mapstructure:"app_secret"`            // 公众号/小程序 AppSecret（OAuth 换 openid 用）
	APIv3Key           string `mapstructure:"api_v3_key"`            // APIv3 密钥（回调解密用）
	MerchantSerialNo   string `mapstructure:"merchant_serial_no"`    // 商户 API 证书序列号
	MerchantPrivateKey string `mapstructure:"merchant_private_key"`  // 商户 API 私钥（PEM 内容）
	PlatformSerialNo   string `mapstructure:"platform_serial_no"`    // 微信平台证书序列号（验签用）
	PlatformPublicKey  string `mapstructure:"platform_public_key"`   // 微信平台证书公钥（PEM 内容）
	NotifyBaseURL      string `mapstructure:"notify_base_url"`       // 回调地址前缀，如 https://checkout.huipay.cn
	BaseURL            string `mapstructure:"base_url"`              // 微信支付 API 基础地址，默认 https://api.mch.weixin.qq.com
}

// Load 从配置文件加载配置，并允许 ENV 覆盖（HUIPAY_* 前缀）。
func Load() *Config {
	v := viper.New()
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath("./configs")
	v.AddConfigPath("./infra/config")
	v.AddConfigPath("/opt/huipay/config")
	v.AddConfigPath(".")

	v.SetEnvPrefix("HUIPAY")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	v.SetDefault("http_port", 8080)
	v.SetDefault("gin_mode", "release")
	v.SetDefault("log_level", "info")
	v.SetDefault("app_name", "huipay-backend")
	v.SetDefault("app_env", "local")
	v.SetDefault("mysql_master", "huipay:huipay@tcp(127.0.0.1:3306)/huipay_main?charset=utf8mb4&parseTime=True&loc=Local")

	if err := v.ReadInConfig(); err != nil {
		fmt.Printf("[config] no config file found, use defaults + ENV: %v\n", err)
	}

	cfg := &Config{}
	if err := v.Unmarshal(cfg); err != nil {
		panic(fmt.Errorf("config unmarshal failed: %w", err))
	}
	return cfg
}