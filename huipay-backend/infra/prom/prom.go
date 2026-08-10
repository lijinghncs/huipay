// 包 prom 暴露 Prometheus 指标和 /metrics handler。
package prom

import (
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	OrderCreateTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "order_create_total", Help: "订单创建总数",
	})
	SplitSuccessRate = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "split_success_rate", Help: "分账成功率（0~1）",
	})
	ChannelLatencySeconds = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:       "channel_latency_seconds", Help: "通道调用延迟（秒）",
		Buckets:    prometheus.DefBuckets,
	}, []string{"method", "endpoint"})
	WalletBalanceMismatch = promauto.NewCounter(prometheus.CounterOpts{
		Name: "wallet_balance_mismatch_total", Help: "钱包余额不一致计数",
	})
	IdempotentHit = promauto.NewCounter(prometheus.CounterOpts{
		Name: "idempotent_hit_total", Help: "幂等命中次数",
	})
)

// MustRegister 注册自定义指标（ProMetrics 已通过 promauto 自动注册）。
func MustRegister() {}

// Handler 返回 /metrics handler。
func Handler() gin.HandlerFunc {
	h := promhttp.Handler()
	return func(c *gin.Context) {
		h.ServeHTTP(c.Writer, c.Request)
	}
}