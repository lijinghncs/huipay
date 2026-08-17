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

	// ===== V2 合并版：分账前置对账 + 每日执行指标 =====
	SplitPrecheckDiffTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "split_precheck_diff_total", Help: "分账前置对账差异总数（按层级：TOTAL/DETAIL/PASS）",
	}, []string{"level"})
	SplitDailyExecTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "split_daily_exec_total", Help: "每日分账执行总数（按状态：SUCCESS/PARTIAL/FAILED）",
	}, []string{"status"})
	SplitDailyExecDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name: "split_daily_exec_duration_ms", Help: "每日分账执行耗时（毫秒）",
		Buckets: []float64{50, 100, 200, 500, 1000, 2000, 5000, 10000},
	})
	SplitStatusRecomputeDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name: "split_status_recompute_duration_ms", Help: "门店×日 split_status 异步汇总耗时（毫秒）",
		Buckets: []float64{10, 30, 60, 100, 200, 500, 1000},
	})

	// ===== V3 分账执行与可观测性增强 =====
	SplitExecutionDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name: "split_execution_duration_ms", Help: "分账执行耗时（毫秒，按终态标签）",
		Buckets: []float64{50, 100, 200, 500, 1000, 2000, 5000, 10000, 30000},
	})
	SplitReceiverCount = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "split_receiver_count", Help: "当前分账接收方数量",
	})
	SplitOutboxPendingCount = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "split_outbox_pending_total", Help: "outbox 待处理事件积压数",
	})
	SplitEventCount = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "split_event_total", Help: "分账领域事件发布数（按事件类型）",
	}, []string{"event_type"})
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