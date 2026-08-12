// 包 config 提供应用配置加载能力，基于 Viper 读取 config.yaml 与 ENV。
package config

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/viper"
)

// Config 应用全局配置。
type Config struct {
	HTTPPort    int    `mapstructure:"http_port"`
	GinMode     string `mapstructure:"gin_mode"`     // debug / release / test
	LogLevel    string `mapstructure:"log_level"`    // debug / info / warn / error
	LogFile     LogFileConfig `mapstructure:"log_file"` // 日志文件输出配置（落盘 + 轮转）
	MySQLMaster string `mapstructure:"mysql_master"` // 主库 DSN
	MySQLSlave  string `mapstructure:"mysql_slave"`  // 从库 DSN（可空）
	AppName     string `mapstructure:"app_name"`
	AppEnv      string `mapstructure:"app_env"` // local / staging / production
	AuthSecret  string `mapstructure:"auth_secret"` // 商户登录 token 签名密钥（生产必配）
	TrustMerchantHeader bool `mapstructure:"trust_merchant_header"` // 是否信任 X-Merchant-Id 明文头（仅开发；生产置 false）
	CheckoutBaseURL string `mapstructure:"checkout_base_url"` // 收银台 H5 地址前缀，如 https://checkout.huipay.cn
	WeChat      WeChatConfig `mapstructure:"wechat"`
}

// LogFileConfig 日志文件输出配置（落盘 + 轮转）。
type LogFileConfig struct {
	Enabled    bool   `mapstructure:"enabled"`     // 是否写入文件
	Path       string `mapstructure:"path"`        // 日志文件路径，如 logs/app.log
	MaxSizeMB  int    `mapstructure:"max_size_mb"` // 单个日志文件最大大小（MB），超过则轮转
	MaxAgeDay  int    `mapstructure:"max_age_day"` // 保留天数
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

	// 基于源码位置动态定位项目根目录（huipay-backend），加入绝对搜索路径，
	// 避免因 go run 工作目录不同而找不到 infra/config/config.yaml。
	if _, srcFile, _, ok := runtime.Caller(0); ok {
		// srcFile = <root>/infra/config/config.go，上两级即项目根
		root := filepath.Dir(filepath.Dir(filepath.Dir(srcFile)))
		v.AddConfigPath(filepath.Join(root, "infra", "config"))
		v.AddConfigPath(filepath.Join(root, "configs"))
	}
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
	v.SetDefault("trust_merchant_header", true)
	v.SetDefault("checkout_base_url", "https://checkout.huipay.cn")
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
