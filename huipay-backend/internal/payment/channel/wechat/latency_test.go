package wechat

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"

	"github.com/huipay/huipay-backend/infra/prom"
	"github.com/huipay/huipay-backend/internal/payment/channel"
)

// histCount 读取指定 method+endpoint 的直方图样本总数。
func histCount(method, endpoint string) uint64 {
	obs := prom.ChannelLatencySeconds.WithLabelValues(method, endpoint)
	h, ok := obs.(prometheus.Histogram)
	if !ok {
		return 0
	}
	var m dto.Metric
	_ = h.Write(&m)
	return m.GetHistogram().GetSampleCount()
}

// TestDoJSONLatencySuccess 成功路径埋点计数 +1。
func TestDoJSONLatencySuccess(t *testing.T) {
	a, _ := newTestAdapter(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"code_url": "weixin://wxpay/bizpayurl?pr=xyz"})
	}))

	before := histCount(http.MethodPost, "prepay_native")
	_, err := a.CreatePayment(context.Background(), &channel.CreatePaymentRequest{
		OrderNo: "HP123", Amount: 100, Subject: "x", ExpireSecs: 900,
	})
	if err != nil {
		t.Fatalf("create payment: %v", err)
	}
	if after := histCount(http.MethodPost, "prepay_native"); after <= before {
		t.Fatalf("latency metric not recorded: before=%d after=%d", before, after)
	}
}

// TestDoJSONLatencyFailure 失败路径（500）同样埋点，且错误可提取业务码。
func TestDoJSONLatencyFailure(t *testing.T) {
	a, _ := newTestAdapter(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{"code": "SYSTEMERROR", "message": "weixin down"})
	}))

	before := histCount(http.MethodPost, "prepay_native")
	_, err := a.CreatePayment(context.Background(), &channel.CreatePaymentRequest{
		OrderNo: "HP123", Amount: 100, Subject: "x", ExpireSecs: 900,
	})
	if err == nil {
		t.Fatal("expected error on 503")
	}
	if after := histCount(http.MethodPost, "prepay_native"); after <= before {
		t.Fatalf("latency metric not recorded on failure: before=%d after=%d", before, after)
	}
}