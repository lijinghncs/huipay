// 告警通知器单元测试。
package notify

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"
)

// TestNoopAlerter 空实现不 panic。
func TestNoopAlerter(t *testing.T) {
	NoopAlerter{}.Alert(context.Background(), "t", "c")
}

// TestNewEmptyURL 空 URL 返回 Noop。
func TestNewEmptyURL(t *testing.T) {
	if _, ok := New("", zap.NewNop()).(NoopAlerter); !ok {
		t.Fatal("empty url should return NoopAlerter")
	}
}

// TestWebhookAlerterSendsMarkdown webhook 收到企业微信 markdown 格式消息。
func TestWebhookAlerterSendsMarkdown(t *testing.T) {
	var called int32
	bodyCh := make(chan []byte, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&called, 1)
		body, _ := io.ReadAll(r.Body)
		bodyCh <- body
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	a := NewWebhookAlerter(srv.URL, zap.NewNop())
	a.Alert(context.Background(), "【分账死单】", "订单号：X123")

	select {
	case body := <-bodyCh:
		var payload map[string]any
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("invalid json: %v", err)
		}
		if payload["msgtype"] != "markdown" {
			t.Fatalf("msgtype mismatch: %v", payload["msgtype"])
		}
		md, ok := payload["markdown"].(map[string]any)
		if !ok || md["content"] == "" {
			t.Fatalf("markdown content missing: %v", payload)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("webhook not called within 3s")
	}
}

// TestWebhookAlerterServerDown 目标不可达时不阻塞不 panic。
func TestWebhookAlerterServerDown(t *testing.T) {
	a := NewWebhookAlerter("http://127.0.0.1:1/unreachable", zap.NewNop())
	done := make(chan struct{})
	go func() {
		a.Alert(context.Background(), "t", "c")
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Alert should not block")
	}
	time.Sleep(100 * time.Millisecond) // 等待异步 goroutine 完成（失败仅记日志）
}
