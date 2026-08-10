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