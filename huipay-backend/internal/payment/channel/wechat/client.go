// 微信支付 V3 HTTP 客户端：构造带签名请求，封装下单/查单/退款/关单。
package wechat

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/huipay/huipay-backend/infra/config"
	"github.com/huipay/huipay-backend/infra/prom"
)

// 微信支付 V3 API 路径。
const (
	pathNativePrepay = "/v3/pay/transactions/native"
	pathJSPrepay     = "/v3/pay/transactions/jsapi"
	pathH5Prepay     = "/v3/pay/transactions/h5"
	pathQueryOrder   = "/v3/pay/transactions/out-trade-no/%s"
	pathCloseOrder   = "/v3/pay/transactions/out-trade-no/%s/close"
	pathRefund       = "/v3/refund/domestic/refunds"
	pathTradeBill    = "/v3/billdownload/tradebill"
)

// Client 微信支付 V3 客户端。
type Client struct {
	cfg     config.WeChatConfig
	httpCli *http.Client
	privKey *rsa.PrivateKey
	platKey *rsa.PublicKey
	baseURL string
}

// NewClient 构造 V3 客户端，解析商户私钥与平台证书公钥。
func NewClient(cfg config.WeChatConfig) (*Client, error) {
	privKey, err := parsePrivateKey(cfg.MerchantPrivateKey)
	if err != nil {
		return nil, err
	}
	var platKey *rsa.PublicKey
	if cfg.PlatformPublicKey != "" {
		platKey, err = parsePublicKey(cfg.PlatformPublicKey)
		if err != nil {
			return nil, err
		}
	}
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = "https://api.mch.weixin.qq.com"
	}
	return &Client{
		cfg:     cfg,
		httpCli: &http.Client{Timeout: 10 * time.Second},
		privKey: privKey,
		platKey: platKey,
		baseURL: baseURL,
	}, nil
}

// Config 返回微信通道配置（供适配器使用）。
func (c *Client) Config() config.WeChatConfig { return c.cfg }

// --- 金额 ---

// Amount 订单金额（单位：分）。
type Amount struct {
	Total    int    `json:"total"`
	Currency string `json:"currency"`
}

// --- 预下单 ---

// NativePrepayRequest Native 扫码下单请求。
type NativePrepayRequest struct {
	AppID       string `json:"appid"`
	MchID       string `json:"mchid"`
	Description string `json:"description"`
	OutTradeNo  string `json:"out_trade_no"`
	NotifyURL   string `json:"notify_url"`
	Amount      Amount `json:"amount"`
	TimeExpire  string `json:"time_expire,omitempty"`
}

// NativePrepayResponse Native 下单响应。
type NativePrepayResponse struct {
	CodeURL string `json:"code_url"`
}

// NativePrepay Native 扫码下单，返回 code_url（用于渲染二维码）。
func (c *Client) NativePrepay(ctx context.Context, req *NativePrepayRequest) (*NativePrepayResponse, error) {
	var out NativePrepayResponse
	if err := c.doJSON(ctx, http.MethodPost, pathNativePrepay, req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// H5PrepayRequest H5 支付下单请求。
type H5PrepayRequest struct {
	AppID       string  `json:"appid"`
	MchID       string  `json:"mchid"`
	Description string  `json:"description"`
	OutTradeNo  string  `json:"out_trade_no"`
	NotifyURL   string  `json:"notify_url"`
	Amount      Amount  `json:"amount"`
	SceneInfo   H5Scene `json:"scene_info"`
	TimeExpire  string  `json:"time_expire,omitempty"`
}

// H5Scene H5 场景信息。
type H5Scene struct {
	Type string `json:"type"` // 取值固定为 "H5"
}

// H5PrepayResponse H5 下单响应。
type H5PrepayResponse struct {
	H5URL string `json:"h5_url"`
}

// H5Prepay H5 支付下单，返回 h5_url（用于移动端跳转）。
func (c *Client) H5Prepay(ctx context.Context, req *H5PrepayRequest) (*H5PrepayResponse, error) {
	var out H5PrepayResponse
	if err := c.doJSON(ctx, http.MethodPost, pathH5Prepay, req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// JSPrepayRequest JSAPI 支付下单请求。
type JSPrepayRequest struct {
	AppID       string `json:"appid"`
	MchID       string `json:"mchid"`
	Description string `json:"description"`
	OutTradeNo  string `json:"out_trade_no"`
	NotifyURL   string `json:"notify_url"`
	Amount      Amount `json:"amount"`
	Payer       Payer  `json:"payer"`
	TimeExpire  string `json:"time_expire,omitempty"`
}

// Payer 支付者信息（JSAPI 需要 openid）。
type Payer struct {
	OpenID string `json:"openid"`
}

// JSPrepayResponse JSAPI 下单响应。
type JSPrepayResponse struct {
	PrepayID string `json:"prepay_id"`
}

// JSPrepay JSAPI 支付下单，返回 prepay_id（用于前端拉起）。
func (c *Client) JSPrepay(ctx context.Context, req *JSPrepayRequest) (*JSPrepayResponse, error) {
	var out JSPrepayResponse
	if err := c.doJSON(ctx, http.MethodPost, pathJSPrepay, req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// --- 查单 ---

// QueryOrderResponse 查单响应。
type QueryOrderResponse struct {
	AppID         string      `json:"appid"`
	MchID         string      `json:"mchid"`
	OutTradeNo    string      `json:"out_trade_no"`
	TransactionID string      `json:"transaction_id"`
	TradeState    string      `json:"trade_state"` // SUCCESS / REFUND / NOTPAY / CLOSED / ...
	SuccessTime   string      `json:"success_time"`
	Amount        QueryAmount `json:"amount"`
}

// QueryAmount 查单金额。
type QueryAmount struct {
	Total      int    `json:"total"`
	PayerTotal int    `json:"payer_total"`
	Currency   string `json:"currency"`
}

// QueryOrder 按商户订单号查单。
func (c *Client) QueryOrder(ctx context.Context, outTradeNo string) (*QueryOrderResponse, error) {
	path := fmt.Sprintf(pathQueryOrder+"?mchid=%s", url.PathEscape(outTradeNo), c.cfg.MchID)
	var out QueryOrderResponse
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// --- 关单 ---

// CloseOrder 关闭未支付订单。
func (c *Client) CloseOrder(ctx context.Context, outTradeNo string) error {
	path := fmt.Sprintf(pathCloseOrder, url.PathEscape(outTradeNo))
	return c.doJSON(ctx, http.MethodPost, path, struct{}{}, nil)
}

// --- 退款 ---

// RefundRequest 退款请求。
type RefundRequest struct {
	OutTradeNo    string       `json:"out_trade_no,omitempty"`
	TransactionID string       `json:"transaction_id,omitempty"`
	OutRefundNo   string       `json:"out_refund_no"`
	Reason        string       `json:"reason,omitempty"`
	NotifyURL     string       `json:"notify_url,omitempty"`
	Amount        RefundAmount `json:"amount"`
}

// RefundAmount 退款金额。
type RefundAmount struct {
	Refund   int    `json:"refund"`
	Total    int    `json:"total"`
	Currency string `json:"currency"`
}

// RefundResponse 退款响应。
type RefundResponse struct {
	RefundID      string `json:"refund_id"`
	OutRefundNo   string `json:"out_refund_no"`
	Channel       string `json:"channel"`
	Status        string `json:"status"` // SUCCESS / CLOSED / PROCESSING / ABNORMAL
	TransactionID string `json:"transaction_id"`
	OutTradeNo    string `json:"out_trade_no"`
}

// Refund 发起退款。
func (c *Client) Refund(ctx context.Context, req *RefundRequest) (*RefundResponse, error) {
	var out RefundResponse
	if err := c.doJSON(ctx, http.MethodPost, pathRefund, req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// --- 对账单下载 ---

// TradeBillResponse 交易对账单下载接口响应。
type TradeBillResponse struct {
	DownloadURL string `json:"download_url"`
	Hash        string `json:"hash"` // 文件内容的 SHA256（用于完整性校验）
	HashType    string `json:"hash_type"`
	Nonce       string `json:"nonce"`
	Timestamp   string `json:"timestamp"`
}

// TradeBill 申请下载某日的交易对账单，返回 download_url。
// date 格式为 yyyy-MM-dd。
func (c *Client) TradeBill(ctx context.Context, date string) (*TradeBillResponse, error) {
	path := fmt.Sprintf("%s?bill_date=%s&bill_type=ALL", pathTradeBill, url.QueryEscape(date))
	var out TradeBillResponse
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DownloadFile 下载微信返回的文件流（对账单），DownloadURL 为跨域完整地址。
// 下载同样需要 V3 签名与 Accept: application/gzip。
func (c *Client) DownloadFile(ctx context.Context, rawURL string) (io.ReadCloser, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("wechat: parse download url: %w", err)
	}
	canonicalURL := u.RequestURI() // path + query，签名用

	// 手动构造签名（download_url 非 baseURL 域名，不能复用 newRequest）
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	nonce := randomNonce()
	signStr := buildRequestSignStr(http.MethodGet, canonicalURL, ts, nonce, "")
	sig, err := rsaSHA256Sign(c.privKey, signStr)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("wechat: new download request: %w", err)
	}
	req.Header.Set("Accept", "application/gzip")
	req.Header.Set("Authorization", buildAuthHeader(c.cfg.MchID, c.cfg.MerchantSerialNo, ts, nonce, sig))

	resp, err := c.httpCli.Do(req)
	if err != nil {
		return nil, fmt.Errorf("wechat: download bill %s fail: %w", rawURL, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("wechat: download bill %s status %d: %s", rawURL, resp.StatusCode, string(body))
	}
	return resp.Body, nil
}

// --- 内部请求封装 ---

// apiError 微信 API 错误响应（非 2xx 时返回）。
type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// doJSON 发送带签名的 JSON 请求并解析响应。
// path 需为完整路径（含 query，如 /v3/pay/transactions/out-trade-no/x?mchid=y）。
func (c *Client) doJSON(ctx context.Context, method, path string, body any, out any) error {
	start := time.Now()
	// defer 在函数返回前执行，无论成功/失败都埋点
	defer func() {
		prom.ChannelLatencySeconds.
			WithLabelValues(method, pathCategory(path)).
			Observe(time.Since(start).Seconds())
	}()

	var payload []byte
	var err error
	if body != nil {
		payload, err = json.Marshal(body)
		if err != nil {
			return fmt.Errorf("wechat: marshal body: %w", err)
		}
	}

	req, err := c.newRequest(ctx, method, path, payload)
	if err != nil {
		return err
	}

	resp, err := c.httpCli.Do(req)
	if err != nil {
		return fmt.Errorf("wechat: request %s fail: %w", path, err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		if out != nil && len(respBody) > 0 {
			if err := json.Unmarshal(respBody, out); err != nil {
				return fmt.Errorf("wechat: decode resp %s: %w", path, err)
			}
		}
		return nil
	}

	var ae apiError
	_ = json.Unmarshal(respBody, &ae)
	if ae.Message == "" {
		ae.Message = string(respBody)
	}
	bizErr := MapErr(resp.StatusCode, ae)
	return fmt.Errorf("%w (raw: %d %s)", bizErr, resp.StatusCode, path)
}

// pathCategory 将 path 归类到有限 endpoint 标签，避免 Prometheus cardinality 爆炸。
// 例如：/v3/pay/transactions/out-trade-no/HP1 → "query"。
func pathCategory(path string) string {
	switch {
	case strings.HasSuffix(path, "/native"):
		return "prepay_native"
	case strings.HasSuffix(path, "/h5"):
		return "prepay_h5"
	case strings.HasSuffix(path, "/jsapi"):
		return "prepay_jsapi"
	case strings.HasSuffix(path, "/close"):
		return "close"
	case strings.HasPrefix(path, "/v3/pay/transactions/out-trade-no/"):
		return "query"
	case strings.HasSuffix(path, "/refunds"):
		return "refund"
	case strings.HasSuffix(path, "/certificates"):
		return "certificates"
	case strings.HasPrefix(path, "/v3/billdownload/"):
		return "bill_download"
	default:
		return "other"
	}
}

// newRequest 构造带 V3 签名的请求。
func (c *Client) newRequest(ctx context.Context, method, path string, payload []byte) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("wechat: new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "huipay-backend/1.0")
	if req.Method == http.MethodGet && len(payload) == 0 {
		req.ContentLength = 0
	}

	ts := strconv.FormatInt(time.Now().Unix(), 10)
	nonce := randomNonce()
	signStr := buildRequestSignStr(method, path, ts, nonce, string(payload))
	sig, err := rsaSHA256Sign(c.privKey, signStr)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", buildAuthHeader(c.cfg.MchID, c.cfg.MerchantSerialNo, ts, nonce, sig))
	return req, nil
}

// randomNonce 生成 16 字节随机串（hex 编码）。
func randomNonce() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}