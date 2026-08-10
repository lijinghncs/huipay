package reconcile

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/huipay/huipay-backend/infra/config"
)

// genPEM 生成一对 RSA 密钥 PEM（用于构造下载器）。
func genPEM(t *testing.T) (privPEM, pubPEM string) {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("gen rsa key: %v", err)
	}
	privPEM = string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)}))
	pubASN1, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	if err != nil {
		t.Fatalf("marshal pub: %v", err)
	}
	pubPEM = string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubASN1}))
	return privPEM, pubPEM
}

// gzipCSV 将 CSV 行压缩为 gzip 字节流。
func gzipCSV(rows []string) []byte {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	_, _ = gz.Write([]byte(strings.Join(rows, "\n")))
	_ = gz.Close()
	return buf.Bytes()
}

// newMockDownloader 起一个 mock 微信接口 server：
// /v3/billdownload/tradebill 返回 download_url 指向 /bill，/bill 返回给定 gzip CSV。
func newMockDownloader(t *testing.T, bill []byte) (*Downloader, *httptest.Server) {
	t.Helper()
	privPEM, pubPEM := genPEM(t)
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/v3/billdownload/tradebill"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(fmt.Sprintf(`{"download_url": %q, "hash": "x", "hash_type": "SHA256"}`, srv.URL+"/bill")))
		case r.URL.Path == "/bill":
			w.Header().Set("Content-Type", "application/gzip")
			_, _ = w.Write(bill)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	d, err := NewDownloader(config.WeChatConfig{
		Enabled:            true,
		MchID:              "mch_1001",
		AppID:              "app_2002",
		APIv3Key:           "0123456789abcdef0123456789abcdef",
		MerchantSerialNo:   "serial_abc",
		MerchantPrivateKey: privPEM,
		PlatformPublicKey:  pubPEM,
		BaseURL:            srv.URL,
	})
	if err != nil {
		t.Fatalf("new downloader: %v", err)
	}
	return d, srv
}

// billLine 构造一行微信交易对账单 CSV（打满 24+ 列，金额在第 23 列）。
func billLine(tradeTime, txnID, outTradeNo, amount string) string {
	cols := make([]string, 24)
	cols[0] = tradeTime
	cols[4] = txnID
	cols[5] = outTradeNo
	cols[7] = "JSAPI"
	cols[8] = "SUCCESS"
	cols[23] = amount
	return strings.Join(cols, ",")
}

const billHeader = "交易时间,商户号,特约商户号,服务商商户号,微信订单号,商户订单号,用户openid,交易类型,交易状态,付款银行,货币种类,应结订单金额,代金券金额,退款金额,充值券退款金额,手续费金额,费率,订单金额,申请退款金额,费率备注,商户实收金额,商户退款金额,商户优惠金额,留存余额"

// TestParseBill 正常解析一行。
func TestParseBill(t *testing.T) {
	d, _ := newMockDownloader(t, gzipCSV([]string{
		billHeader,
		billLine("2026-08-10 10:00:00", "TXN1001", "HP0001", "100"),
	}))
	entries, err := d.DownloadBill(context.Background(), "2026-08-10")
	if err != nil {
		t.Fatalf("download bill: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("len(entries) = %d, want 1", len(entries))
	}
	e := entries[0]
	if e.TransactionID != "TXN1001" || e.OutTradeNo != "HP0001" || e.OrderAmount != 100 {
		t.Fatalf("unexpected entry: %+v", e)
	}
}

// TestParseBillGzipEmpty 空 gzip 文件：无条目不报错。
func TestParseBillGzipEmpty(t *testing.T) {
	d, _ := newMockDownloader(t, gzipCSV(nil))
	entries, err := d.DownloadBill(context.Background(), "2026-08-10")
	if err != nil {
		t.Fatalf("download empty bill: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected 0 entries, got %d", len(entries))
	}
}

// TestParseBillCorrupt 损坏的 gzip 文件：返回错误。
func TestParseBillCorrupt(t *testing.T) {
	d, _ := newMockDownloader(t, []byte("this is not gzip"))
	_, err := d.DownloadBill(nil, "2026-08-10")
	if err == nil {
		t.Fatal("expected error for corrupt gzip")
	}
}

// TestParseBillSingleLine 单行有效数据 + 汇总行跳过。
func TestParseBillSingleLine(t *testing.T) {
	d, _ := newMockDownloader(t, gzipCSV([]string{
		billLine("2026-08-10 10:00:00", "TXN1001", "HP0001", "100"),
		"总交易单数,1,,,,,,,,,,,,,,,,,,,,,,,",
	}))
	entries, err := d.DownloadBill(context.Background(), "2026-08-10")
	if err != nil {
		t.Fatalf("download bill: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("len(entries) = %d, want 1 (summary skipped)", len(entries))
	}
}