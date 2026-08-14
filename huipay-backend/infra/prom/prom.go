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
	PaymentCodeInvalidAttempts = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "payment_code_invalid_attempts_total",
		Help: "码牌异常请求计数（按原因：not_found / disabled / amount_out_of_range）",
	}, []string{"reason"})

	// ===== 分账容错指标（E 层）=====
	SplitAmountTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "split_amount_total", Help: "分账金额累计（分）",
	})
	SplitOrderTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "split_order_total", Help: "分账订单数（按终态）",
	}, []string{"status"})
	SplitFailureReasonTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "split_failure_reason_total", Help: "分账失败原因计数",
	}, []string{"reason"})
	SplitRetryTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "split_retry_total", Help: "补偿重试次数",
	})
	SplitHangingTotal = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "split_hanging_total", Help: "悬挂分账订单数",
	})
	SplitAutoDisabledTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "split_auto_disabled_total", Help: "自动分账熔断触发次数",
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
