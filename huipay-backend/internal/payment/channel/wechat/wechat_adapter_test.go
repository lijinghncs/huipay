package wechat

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/huipay/huipay-backend/infra/config"
	"github.com/huipay/huipay-backend/internal/payment/channel"
)

// newTestAdapter 起一个模拟微信 V3 接口的 httptest server，并构造适配器。
func newTestAdapter(t *testing.T, handler http.Handler) (*Adapter, *httptest.Server) {
	t.Helper()
	privPEM, pubPEM := genKeyPEM(t)
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	cfg := config.WeChatConfig{
		Enabled:            true,
		MchID:              "mch_1001",
		AppID:              "app_2002",
		APIv3Key:           "0123456789abcdef0123456789abcdef",
		MerchantSerialNo:   "serial_abc",
		MerchantPrivateKey: privPEM,
		PlatformSerialNo:   "serial_plat",
		PlatformPublicKey:  pubPEM,
		NotifyBaseURL:      "https://checkout.huipay.cn",
		BaseURL:            srv.URL,
	}
	a, err := New(cfg)
	if err != nil {
		t.Fatalf("new adapter: %v", err)
	}
	return a, srv
}

// capture 记录最近一次收到的请求，便于断言。
type capture struct {
	mu      sync.Mutex
	path    string
	method  string
	auth    string
	body    string
}

func (c *capture) handle(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		c.mu.Lock()
		c.path = r.URL.Path
		c.method = r.Method
		c.auth = r.Header.Get("Authorization")
		c.body = string(b)
		c.mu.Unlock()
		handler(w, r)
	}
}

func (c *capture) snapshot() (path, method, auth, body string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.path, c.method, c.auth, c.body
}

func TestCreatePaymentNative(t *testing.T) {
	cap := &capture{}
	a, _ := newTestAdapter(t, cap.handle(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"code_url": "weixin://wxpay/bizpayurl?pr=xyz"})
	}))

	resp, err := a.CreatePayment(context.Background(), &channel.CreatePaymentRequest{
		OrderNo: "HP123", Amount: 100, Subject: "测试商品", ExpireSecs: 900,
	})
	if err != nil {
		t.Fatalf("create native: %v", err)
	}
	if resp.QRCode != "weixin://wxpay/bizpayurl?pr=xyz" {
		t.Fatalf("unexpected qrcode: %q", resp.QRCode)
	}
	_, method, auth, body := cap.snapshot()
	if method != http.MethodPost {
		t.Fatalf("method = %s", method)
	}
	if !strings.Contains(auth, "WECHATPAY2-SHA256-RSA2048") {
		t.Fatalf("missing auth header: %s", auth)
	}
	for _, want := range []string{`"out_trade_no":"HP123"`, `"total":100`, `"notify_url":"https://checkout.huipay.cn/v1/notify/wechat"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("body missing %q: %s", want, body)
		}
	}
}

func TestCreatePaymentH5(t *testing.T) {
	cap := &capture{}
	a, _ := newTestAdapter(t, cap.handle(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"h5_url": "https://wx.tenpay.com/h5/abc"})
	}))

	resp, err := a.CreatePayment(context.Background(), &channel.CreatePaymentRequest{
		OrderNo: "HP456", Amount: 200, Subject: "H5商品", PayType: channel.PayTypeH5,
	})
	if err != nil {
		t.Fatalf("create h5: %v", err)
	}
	if resp.PayURL != "https://wx.tenpay.com/h5/abc" {
		t.Fatalf("unexpected h5_url: %q", resp.PayURL)
	}
	path, _, _, body := cap.snapshot()
	if !strings.HasSuffix(path, "/h5") {
		t.Fatalf("h5 path = %s", path)
	}
	if !strings.Contains(body, `"type":"H5"`) {
		t.Fatalf("h5 scene missing: %s", body)
	}
}

func TestCreatePaymentJSAPI(t *testing.T) {
	cap := &capture{}
	a, _ := newTestAdapter(t, cap.handle(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"prepay_id": "wx111"})
	}))

	resp, err := a.CreatePayment(context.Background(), &channel.CreatePaymentRequest{
		OrderNo: "HP789", Amount: 300, Subject: "JSAPI", PayType: channel.PayTypeJSAPI, OpenID: "open_1",
	})
	if err != nil {
		t.Fatalf("create jsapi: %v", err)
	}
	if resp.PrepayID != "wx111" {
		t.Fatalf("unexpected prepay_id: %q", resp.PrepayID)
	}
	_, _, _, body := cap.snapshot()
	if !strings.Contains(body, `"openid":"open_1"`) {
		t.Fatalf("jsapi payer openid missing: %s", body)
	}
}

func TestCreatePaymentJSAPIRequiresOpenID(t *testing.T) {
	a, _ := newTestAdapter(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"prepay_id": "wx111"})
	}))
	_, err := a.CreatePayment(context.Background(), &channel.CreatePaymentRequest{
		OrderNo: "HP789", Amount: 300, PayType: channel.PayTypeJSAPI,
	})
	if err == nil {
		t.Fatal("jsapi without openid should fail")
	}
}

func TestQueryPayment(t *testing.T) {
	a, _ := newTestAdapter(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"transaction_id": "4200000001",
			"trade_state":    "SUCCESS",
			"success_time":   "2026-08-10T10:00:00+08:00",
			"amount":         map[string]any{"total": 100, "payer_total": 100, "currency": "CNY"},
		})
	}))

	status, err := a.QueryPayment(context.Background(), "HP123")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if !status.Paid {
		t.Fatal("expect paid")
	}
	if status.ChannelTradeNo != "4200000001" {
		t.Fatalf("trade no = %q", status.ChannelTradeNo)
	}
	if status.PaidAmount != 100 {
		t.Fatalf("paid amount = %d", status.PaidAmount)
	}
	if status.PaidAt == 0 {
		t.Fatal("paid_at should be parsed")
	}
}

func TestQueryPaymentNotPaid(t *testing.T) {
	a, _ := newTestAdapter(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"trade_state": "NOTPAY"})
	}))
	status, err := a.QueryPayment(context.Background(), "HP123")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if status.Paid {
		t.Fatal("expect not paid")
	}
}

func TestRefund(t *testing.T) {
	cap := &capture{}
	a, _ := newTestAdapter(t, cap.handle(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"refund_id": "rf_999", "status": "SUCCESS"})
	}))

	resp, err := a.Refund(context.Background(), &channel.RefundRequest{
		OrderNo: "HP123", RefundNo: "RF123", RefundAmount: 50, TotalAmount: 100, Reason: "测试退款",
	})
	if err != nil {
		t.Fatalf("refund: %v", err)
	}
	if resp.ChannelRefundNo != "rf_999" {
		t.Fatalf("refund no = %q", resp.ChannelRefundNo)
	}
	path, _, _, body := cap.snapshot()
	if !strings.HasSuffix(path, "/refunds") {
		t.Fatalf("refund path = %s", path)
	}
	for _, want := range []string{`"out_trade_no":"HP123"`, `"out_refund_no":"RF123"`, `"refund":50`, `"total":100`} {
		if !strings.Contains(body, want) {
			t.Fatalf("refund body missing %q: %s", want, body)
		}
	}
}

func TestSplitNotImplemented(t *testing.T) {
	a, _ := newTestAdapter(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	if _, err := a.Split(context.Background(), &channel.SplitRequest{OrderNo: "HP1"}); err == nil {
		t.Fatal("split should be not implemented")
	}
}

// buildNotifyEnvelope 构造微信回调信封（用测试密钥加密交易详情）。
// nonce 固定为可打印 12 字节，保证 JSON 合法且可作为 GCM nonce。
func buildNotifyEnvelope(t *testing.T, apiV3Key, plaintext string) (raw string, aad string, nonce string) {
	t.Helper()
	aad = "transaction"
	nonce = "0123456789ab"
	cipherB64 := aesGCMEncrypt(t, []byte(apiV3Key), []byte(aad), []byte(nonce), []byte(plaintext))
	raw = fmt.Sprintf(`{"resource":{"algorithm":"AEAD_AES_256_GCM","ciphertext":"%s","associated_data":"%s","nonce":"%s","original_type":"transaction"}}`,
		cipherB64, aad, nonce)
	return raw, aad, nonce
}

// signNotifyBody 用测试私钥对回调签名串签名（模拟微信平台）。
func signNotifyBody(t *testing.T, privPEM, timestamp, nonce, body string) string {
	t.Helper()
	priv, err := parsePrivateKey(privPEM)
	if err != nil {
		t.Fatalf("parse private: %v", err)
	}
	sig, err := rsaSHA256Sign(priv, buildVerifySignStr(timestamp, nonce, body))
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	return sig
}

func TestVerifyNotify(t *testing.T) {
	privPEM, pubPEM := genKeyPEM(t)
	apiV3Key := "0123456789abcdef0123456789abcdef"
	plaintext := `{"out_trade_no":"HP123","transaction_id":"4200000001","trade_state":"SUCCESS","amount":{"payer_total":100,"total":100}}`
	raw, _, _ := buildNotifyEnvelope(t, apiV3Key, plaintext)

	timestamp := "1700000000"
	nonce := "verify-nonce"
	sig := signNotifyBody(t, privPEM, timestamp, nonce, raw)

	cfg := config.WeChatConfig{APIv3Key: apiV3Key, PlatformPublicKey: pubPEM, MerchantPrivateKey: privPEM}
	a, err := New(cfg)
	if err != nil {
		t.Fatalf("new adapter: %v", err)
	}
	payload, err := a.VerifyNotify(context.Background(), []byte(raw), map[string]string{
		"Wechatpay-Signature": sig,
		"Wechatpay-Timestamp": timestamp,
		"Wechatpay-Nonce":     nonce,
	})
	if err != nil {
		t.Fatalf("verify notify: %v", err)
	}
	if payload.OrderNo != "HP123" || payload.ChannelTradeNo != "4200000001" {
		t.Fatalf("payload = %+v", payload)
	}
	if !payload.Paid || payload.PaidAmount != 100 {
		t.Fatalf("payload paid/amount = %+v", payload)
	}
}

func TestVerifyNotifyTampered(t *testing.T) {
	privPEM, pubPEM := genKeyPEM(t)
	apiV3Key := "0123456789abcdef0123456789abcdef"
	raw, _, _ := buildNotifyEnvelope(t, apiV3Key, `{"out_trade_no":"HP1"}`)
	timestamp := "1700000000"
	nonce := "verify-nonce"
	// 对原始 body 签名，但请求时篡改信封中的 associated_data
	sig := signNotifyBody(t, privPEM, timestamp, nonce, raw)

	a, err := New(config.WeChatConfig{APIv3Key: apiV3Key, PlatformPublicKey: pubPEM, MerchantPrivateKey: privPEM})
	if err != nil {
		t.Fatalf("new adapter: %v", err)
	}
	tampered := strings.Replace(raw, `"associated_data":"transaction"`, `"associated_data":"tampered"`, 1)
	if _, err := a.VerifyNotify(context.Background(), []byte(tampered), map[string]string{
		"Wechatpay-Signature": sig,
		"Wechatpay-Timestamp": timestamp,
		"Wechatpay-Nonce":     nonce,
	}); err == nil {
		t.Fatal("tampered body should fail verify")
	}
}

func TestVerifyNotifyMissingHeaders(t *testing.T) {
	privPEM, pubPEM := genKeyPEM(t)
	a, err := New(config.WeChatConfig{APIv3Key: "0123456789abcdef0123456789abcdef", PlatformPublicKey: pubPEM, MerchantPrivateKey: privPEM})
	if err != nil {
		t.Fatalf("new adapter: %v", err)
	}
	if _, err := a.VerifyNotify(context.Background(), []byte(`{}`), map[string]string{}); err == nil {
		t.Fatal("missing signature headers should fail")
	}
}

func TestVerifyNotifyNoPlatformKey(t *testing.T) {
	privPEM, _ := genKeyPEM(t)
	a, err := New(config.WeChatConfig{APIv3Key: "0123456789abcdef0123456789abcdef", MerchantPrivateKey: privPEM})
	if err != nil {
		t.Fatalf("new adapter: %v", err)
	}
	if _, err := a.VerifyNotify(context.Background(), []byte(`{}`), map[string]string{}); err == nil {
		t.Fatal("missing platform key should fail")
	}
}

func TestCloseOrder(t *testing.T) {
	cap := &capture{}
	a, _ := newTestAdapter(t, cap.handle(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	if err := a.CloseOrder(context.Background(), "HP999"); err != nil {
		t.Fatalf("close order: %v", err)
	}
	path, method, _, _ := cap.snapshot()
	if !strings.HasSuffix(path, "/out-trade-no/HP999/close") {
		t.Fatalf("close path = %s", path)
	}
	if method != http.MethodPost {
		t.Fatalf("close method = %s", method)
	}
}

func TestVerifyAndDecrypt(t *testing.T) {
	privPEM, pubPEM := genKeyPEM(t)
	apiV3Key := "0123456789abcdef0123456789abcdef"
	plaintext := `{"out_trade_no":"HP123","refund_status":"SUCCESS"}`
	raw, _, _ := buildNotifyEnvelope(t, apiV3Key, plaintext)
	timestamp := "1700000000"
	nonce := "verify-nonce"
	sig := signNotifyBody(t, privPEM, timestamp, nonce, raw)

	a, err := New(config.WeChatConfig{APIv3Key: apiV3Key, PlatformPublicKey: pubPEM, MerchantPrivateKey: privPEM})
	if err != nil {
		t.Fatalf("new adapter: %v", err)
	}
	got, err := a.VerifyAndDecrypt(context.Background(), []byte(raw), map[string]string{
		"Wechatpay-Signature": sig,
		"Wechatpay-Timestamp": timestamp,
		"Wechatpay-Nonce":     nonce,
	})
	if err != nil {
		t.Fatalf("verify and decrypt: %v", err)
	}
	var parsed struct {
		OutTradeNo string `json:"out_trade_no"`
	}
	if err := json.Unmarshal(got, &parsed); err != nil {
		t.Fatalf("parse plaintext: %v", err)
	}
	if parsed.OutTradeNo != "HP123" {
		t.Fatalf("plaintext out_trade_no = %q", parsed.OutTradeNo)
	}
}