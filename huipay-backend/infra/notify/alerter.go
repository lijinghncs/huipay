// 包 notify 提供告警通知能力（webhook 机器人）。
package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"go.uber.org/zap"
)

// Alerter 告警通知接口。实现必须非阻塞（异步发送），失败仅记日志不影响主流程。
type Alerter interface {
	Alert(ctx context.Context, title, content string)
}

// NoopAlerter 空实现（未配置 webhook 时使用）。
type NoopAlerter struct{}

// Alert 空操作。
func (NoopAlerter) Alert(context.Context, string, string) {}

// WebhookAlerter 企业微信群机器人 webhook 告警（markdown 消息）。
type WebhookAlerter struct {
	url    string
	client *http.Client
	logger *zap.Logger
}

// New 按配置构造告警器：url 为空返回 NoopAlerter。
func New(url string, logger *zap.Logger) Alerter {
	if url == "" {
		return NoopAlerter{}
	}
	return NewWebhookAlerter(url, logger)
}

// NewWebhookAlerter 构造 webhook 告警器。
func NewWebhookAlerter(url string, logger *zap.Logger) *WebhookAlerter {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &WebhookAlerter{
		url:    url,
		client: &http.Client{Timeout: 5 * time.Second},
		logger: logger,
	}
}

// Alert 异步发送告警（goroutine + recover 兜底，永不阻塞/击穿主流程）。
func (w *WebhookAlerter) Alert(_ context.Context, title, content string) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				w.logger.Warn("alert webhook panic", zap.Any("panic", r))
			}
		}()
		payload := map[string]any{
			"msgtype": "markdown",
			"markdown": map[string]string{
				"content": fmt.Sprintf("## %s\n> %s", title, content),
			},
		}
		body, err := json.Marshal(payload)
		if err != nil {
			w.logger.Warn("alert marshal fail", zap.Error(err))
			return
		}
		req, err := http.NewRequest(http.MethodPost, w.url, bytes.NewReader(body))
		if err != nil {
			w.logger.Warn("alert build request fail", zap.Error(err))
			return
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := w.client.Do(req)
		if err != nil {
			w.logger.Warn("alert webhook send fail", zap.String("title", title), zap.Error(err))
			return
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode >= 300 {
			w.logger.Warn("alert webhook non-2xx", zap.String("title", title), zap.Int("status", resp.StatusCode))
		}
	}()
}
