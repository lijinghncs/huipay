// 包 wechat 实现微信支付 V3 通道适配器。
// 基于迭代 0 的 V3 客户端，按场景（Native/H5/JSAPI）完成预下单、查单、退款。
package wechat

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/huipay/huipay-backend/infra/config"
	"github.com/huipay/huipay-backend/internal/domain/vo"
	"github.com/huipay/huipay-backend/internal/payment/channel"
)

// 默认回调路径（挂载于 NotifyBaseURL 之后）。
const defaultNotifyPath = "/v1/notify/wechat"

// Adapter 微信支付适配器。
type Adapter struct {
	client       *Client
	cfg          config.WeChatConfig
	certProvider CertProvider
	notifyPath   string // 回调路径；商户级适配器带 :merchant_id，用于回调分流
}

// New 构造微信适配器，解析商户证书并初始化 V3 客户端。
// 商户私钥/平台公钥缺失时返回错误，避免通道在未配置时被错误启用。
func New(cfg config.WeChatConfig) (*Adapter, error) {
	return newAdapter(cfg, defaultNotifyPath)
}

// NewForMerchant 构造商户级微信适配器：回调路径携带 merchant_id，供微信回调按商户分流。
func NewForMerchant(cfg config.WeChatConfig, merchantID uint64) (*Adapter, error) {
	return newAdapter(cfg, fmt.Sprintf("/v1/notify/wechat/%d", merchantID))
}

func newAdapter(cfg config.WeChatConfig, notifyPath string) (*Adapter, error) {
	client, err := NewClient(cfg)
	if err != nil {
		return nil, err
	}
	return &Adapter{
		client:       client,
		cfg:          cfg,
		certProvider: &StaticCertProvider{key: client.platKey},
		notifyPath:   notifyPath,
	}, nil
}

// Code 通道编码。
func (a *Adapter) Code() vo.ChannelCode { return vo.ChannelWeChat }

// NotifyPath 返回回调路径（供回调分流诊断/日志使用）。
func (a *Adapter) NotifyPath() string { return a.notifyPath }

// Config 返回适配器使用的运行配置（供回调分流/诊断使用）。
func (a *Adapter) Config() config.WeChatConfig { return a.cfg }

// MerchantID 从回调路径解析商户 ID；平台级适配器返回 0。
func (a *Adapter) MerchantID() uint64 {
	if len(a.notifyPath) <= len(defaultNotifyPath) || a.notifyPath[:len(defaultNotifyPath)] != defaultNotifyPath {
		return 0
	}
	// 路径形如 /v1/notify/wechat/{id}
	id, _ := strconv.ParseUint(a.notifyPath[len(defaultNotifyPath)+1:], 10, 64)
	return id
}

// CreatePayment 预下单：按支付场景分派到 Native/H5/JSAPI。
func (a *Adapter) CreatePayment(ctx context.Context, req *channel.CreatePaymentRequest) (*channel.CreatePaymentResponse, error) {
	payType := req.PayType
	if payType == "" {
		payType = channel.PayTypeNative
	}

	notifyURL := req.NotifyURL
	if notifyURL == "" {
		notifyURL = a.cfg.NotifyBaseURL + a.notifyPath
	}
	timeExpire := ""
	if req.ExpireSecs > 0 {
		timeExpire = time.Now().Add(time.Duration(req.ExpireSecs) * time.Second).Format(time.RFC3339)
	}

	amount := Amount{Total: int(req.Amount), Currency: "CNY"}

	switch payType {
	case channel.PayTypeH5:
		resp, err := a.client.H5Prepay(ctx, &H5PrepayRequest{
			AppID:       a.cfg.AppID,
			MchID:       a.cfg.MchID,
			Description: req.Subject,
			OutTradeNo:  req.OrderNo,
			NotifyURL:   notifyURL,
			Amount:      amount,
			SceneInfo:   H5Scene{Type: "H5"},
			TimeExpire:  timeExpire,
		})
		if err != nil {
			return nil, fmt.Errorf("wechat h5 prepay: %w", err)
		}
		return &channel.CreatePaymentResponse{PayURL: resp.H5URL}, nil

	case channel.PayTypeJSAPI:
		if req.OpenID == "" {
			return nil, fmt.Errorf("wechat jsapi prepay: openid required")
		}
		resp, err := a.client.JSPrepay(ctx, &JSPrepayRequest{
			AppID:       a.cfg.AppID,
			MchID:       a.cfg.MchID,
			Description: req.Subject,
			OutTradeNo:  req.OrderNo,
			NotifyURL:   notifyURL,
			Amount:      amount,
			Payer:       Payer{OpenID: req.OpenID},
			TimeExpire:  timeExpire,
		})
		if err != nil {
			return nil, fmt.Errorf("wechat jsapi prepay: %w", err)
		}
		// 生成前端拉起 JSAPI 所需的调起参数（二次签名）
		jsapiParams, err := BuildJSAPIParams(a.cfg.AppID, resp.PrepayID, a.cfg.APIv3Key)
		if err != nil {
			return nil, fmt.Errorf("wechat jsapi params: %w", err)
		}
		return &channel.CreatePaymentResponse{PrepayID: resp.PrepayID, JSAPIParams: jsapiParams}, nil

	default: // NATIVE
		resp, err := a.client.NativePrepay(ctx, &NativePrepayRequest{
			AppID:       a.cfg.AppID,
			MchID:       a.cfg.MchID,
			Description: req.Subject,
			OutTradeNo:  req.OrderNo,
			NotifyURL:   notifyURL,
			Amount:      amount,
			TimeExpire:  timeExpire,
		})
		if err != nil {
			return nil, fmt.Errorf("wechat native prepay: %w", err)
		}
		return &channel.CreatePaymentResponse{QRCode: resp.CodeURL}, nil
	}
}

// QueryPayment 按商户订单号查单，返回支付状态。
// 入参 channelTradeNo 在本实现中即商户订单号 out_trade_no。
func (a *Adapter) QueryPayment(ctx context.Context, channelTradeNo string) (*channel.PaymentStatus, error) {
	resp, err := a.client.QueryOrder(ctx, channelTradeNo)
	if err != nil {
		return nil, fmt.Errorf("wechat query: %w", err)
	}
	return &channel.PaymentStatus{
		ChannelTradeNo: resp.TransactionID,
		Paid:           resp.TradeState == "SUCCESS",
		PaidAmount:     int64(resp.Amount.PayerTotal),
		PaidAt:         parseTimeSec(resp.SuccessTime),
	}, nil
}

// Refund 发起退款。
func (a *Adapter) Refund(ctx context.Context, req *channel.RefundRequest) (*channel.RefundResponse, error) {
	resp, err := a.client.Refund(ctx, &RefundRequest{
		OutTradeNo: req.OrderNo,
		OutRefundNo: req.RefundNo,
		Reason:     req.Reason,
		NotifyURL:  a.cfg.NotifyBaseURL + "/v1/notify/wechat/refund",
		Amount: RefundAmount{
			Refund:   int(req.RefundAmount),
			Total:    int(req.TotalAmount),
			Currency: "CNY",
		},
	})
	if err != nil {
		return nil, fmt.Errorf("wechat refund: %w", err)
	}
	return &channel.RefundResponse{ChannelRefundNo: resp.RefundID}, nil
}

// Split 微信分账（后续迭代接入 /v3/profitsharing）。
func (a *Adapter) Split(ctx context.Context, req *channel.SplitRequest) (*channel.SplitResponse, error) {
	return nil, fmt.Errorf("wechat split not implemented yet")
}

func (a *Adapter) ReturnSplit(ctx context.Context, req *channel.ReturnSplitRequest) error {
	return fmt.Errorf("wechat return split not implemented yet")
}

func (a *Adapter) FinishSplit(ctx context.Context, req *channel.FinishSplitRequest) error {
	return fmt.Errorf("wechat finish split not implemented yet")
}

// CloseOrder 关闭订单（超时关单定时任务调用）。
func (a *Adapter) CloseOrder(ctx context.Context, orderNo string) error {
	return a.client.CloseOrder(ctx, orderNo)
}

// VerifyAndDecrypt 回调验签 + 解密，返回明文（供退款等非交易回调复用）。
func (a *Adapter) VerifyAndDecrypt(ctx context.Context, raw []byte, headers map[string]string) ([]byte, error) {
	return a.verifyAndDecrypt(ctx, raw, headers)
}

// verifyAndDecrypt 验签 + 解密回调报文，返回解密后的明文。
func (a *Adapter) verifyAndDecrypt(ctx context.Context, raw []byte, headers map[string]string) ([]byte, error) {
	if a.certProvider == nil {
		return nil, fmt.Errorf("wechat: cert provider not configured")
	}
	// 1. 平台证书验签
	signature := headers["Wechatpay-Signature"]
	timestamp := headers["Wechatpay-Timestamp"]
	nonce := headers["Wechatpay-Nonce"]
	if signature == "" || timestamp == "" || nonce == "" {
		return nil, fmt.Errorf("wechat: missing notify signature headers")
	}
	pub, err := a.certProvider.GetBySerial(ctx, headers["Wechatpay-Serial"])
	if err != nil {
		return nil, err
	}
	if err := rsaSHA256Verify(pub, buildVerifySignStr(timestamp, nonce, string(raw)), signature); err != nil {
		return nil, err
	}

	// 2. 解析回调信封
	var envelope struct {
		Resource struct {
			Algorithm      string `json:"algorithm"`
			Ciphertext     string `json:"ciphertext"`
			AssociatedData string `json:"associated_data"`
			Nonce          string `json:"nonce"`
			OriginalType   string `json:"original_type"`
		} `json:"resource"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, fmt.Errorf("wechat: parse notify envelope: %w", err)
	}

	// 3. AES-256-GCM 解密交易详情
	plain, err := aesGCMDecrypt(a.cfg.APIv3Key,
		envelope.Resource.AssociatedData, envelope.Resource.Nonce, envelope.Resource.Ciphertext)
	if err != nil {
		return nil, err
	}
	return plain, nil
}

// VerifyNotify 回调验签 + 解密，提取订单与支付金额。
// headers 需包含 Wechatpay-Signature / Wechatpay-Timestamp / Wechatpay-Nonce。
func (a *Adapter) VerifyNotify(ctx context.Context, raw []byte, headers map[string]string) (*channel.NotifyPayload, error) {
	plain, err := a.verifyAndDecrypt(ctx, raw, headers)
	if err != nil {
		return nil, err
	}

	// 提取回调信封唯一 id（notify_id），供幂等键使用
	var env struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(raw, &env)

	var txn struct {
		OutTradeNo    string `json:"out_trade_no"`
		TransactionID string `json:"transaction_id"`
		TradeState    string `json:"trade_state"`
		Amount        struct {
			PayerTotal int `json:"payer_total"`
			Total      int `json:"total"`
		} `json:"amount"`
	}
	if err := json.Unmarshal(plain, &txn); err != nil {
		return nil, fmt.Errorf("wechat: parse notify resource: %w", err)
	}

	return &channel.NotifyPayload{
		OrderNo:        txn.OutTradeNo,
		ChannelTradeNo: txn.TransactionID,
		NotifyID:       env.ID,
		Paid:           txn.TradeState == "SUCCESS",
		PaidAmount:     int64(txn.Amount.PayerTotal),
		Raw:            plain,
	}, nil
}

// parseTimeSec 解析微信 RFC3339 时间戳为 Unix 秒；解析失败返回 0。
func parseTimeSec(ts string) int64 {
	if ts == "" {
		return 0
	}
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return 0
	}
	return t.Unix()
}
