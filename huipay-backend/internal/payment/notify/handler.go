// 包 notify 处理支付通道回调：验签、解密、幂等入账、金额校验。
package notify

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/huipay/huipay-backend/infra/errs"
	"github.com/huipay/huipay-backend/infra/idem"
	accountsvc "github.com/huipay/huipay-backend/internal/account/service"
	"github.com/huipay/huipay-backend/internal/account/ledger"
	"github.com/huipay/huipay-backend/internal/domain/vo"
	"github.com/huipay/huipay-backend/internal/order/service"
	"github.com/huipay/huipay-backend/internal/payment/channel"
)

// 幂等作用域。
const (
	idemScopeNotify = "wechat:notify"
	idemScopeRefund = "wechat:refund"
	idemExpireDays  = 7 * 24 * time.Hour
)

// retryPolicy 回调错误分级策略。
type retryPolicy int

const (
	retryNo retryPolicy = iota // 业务错误：日志可读但仍走原状态码
	retryYes                   // 系统错误：让微信重试
)

// classifyError 按状态码与业务错误码分级回调错误，用于结构化日志便于排查。
func classifyError(err error, status int) retryPolicy {
	if status >= 500 {
		return retryYes
	}
	// 4xx 默认 retryNo（业务错），但 DB 类错误仍应 500
	var be *errs.BizError
	if errors.As(err, &be) && be.HTTPStatus >= 500 {
		return retryYes
	}
	return retryNo
}

// Handler 支付回调处理器。
type Handler struct {
	wechat             channel.Adapter // 微信通道适配器（未启用时为 nil）
	merchantWechat     MerchantWechatResolver // 商户级微信适配器解析（可空；回调按 :merchant_id 分流）
	order              *service.Service
	accountSvc         *accountsvc.Service
	ledgerSvc          *ledger.Service
	idem               idem.Store
	settlementWalletID uint64 // 微信通道在途资金户 wallet_id（启动时初始化）
	logger             *zap.Logger
}

// MerchantWechatResolver 按商户解析微信适配器（商户级回调验签/解密）。
type MerchantWechatResolver interface {
	Get(ctx context.Context, merchantID uint64) (channel.Adapter, error)
}

// New 构造回调处理器。
func New(
	wechat channel.Adapter,
	merchantWechat MerchantWechatResolver,
	order *service.Service,
	accountSvc *accountsvc.Service,
	ledgerSvc *ledger.Service,
	idemStore idem.Store,
	settlementWalletID uint64,
	logger *zap.Logger,
) *Handler {
	return &Handler{
		wechat:             wechat,
		merchantWechat:     merchantWechat,
		order:              order,
		accountSvc:         accountSvc,
		ledgerSvc:          ledgerSvc,
		idem:               idemStore,
		settlementWalletID: settlementWalletID,
		logger:             logger,
	}
}

// resolveWechat 按回调路径 :merchant_id 选择商户级适配器；无商户参数或解析失败回退平台适配器。
func (h *Handler) resolveWechat(c *gin.Context) channel.Adapter {
	if h.merchantWechat != nil {
		if v := c.Param("merchant_id"); v != "" {
			if id, err := strconv.ParseUint(v, 10, 64); err == nil && id > 0 {
				if a, err := h.merchantWechat.Get(c.Request.Context(), id); err == nil && a != nil {
					return a
				}
				h.logger.Warn("merchant wechat adapter resolve fail, fallback platform",
					zap.Uint64("merchant_id", id),
					zap.String("path", c.Request.URL.Path), zap.Error(err))
			}
		}
	}
	return h.wechat
}

// HandleWechat POST /v1/notify/wechat。
// 成功返回 200（微信停止重试）；校验失败返回 4xx/5xx（微信按策略重试）。
func (h *Handler) HandleWechat(c *gin.Context) {
	adapter := h.resolveWechat(c)
	if adapter == nil {
		h.logger.Warn("wechat notify received but channel disabled")
		c.AbortWithStatus(http.StatusServiceUnavailable)
		return
	}

	raw, err := io.ReadAll(c.Request.Body)
	if err != nil {
		h.logger.Warn("read wechat notify body", zap.Error(err))
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}

	headers := extractNotifyHeaders(c)

	payload, err := adapter.VerifyNotify(c.Request.Context(), raw, headers)
	if err != nil {
		h.logger.Warn("wechat notify verify fail",
			zap.String("trace_id", c.GetString("trace_id")), zap.Error(err))
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}

	// 外层幂等：同一 notify_id 已处理过则直接返回成功，避免重复入账
	if h.idemHit(c.Request.Context(), idemScopeNotify, payload.NotifyID) {
		h.ok(c)
		return
	}

	// 仅处理支付成功事件；其余状态（如退款）返回成功但不入账
	if !payload.Paid {
		h.logger.Info("wechat notify non-success event", zap.String("order_no", payload.OrderNo))
		h.ok(c)
		return
	}

	order, err := h.order.GetByOrderNo(c.Request.Context(), payload.OrderNo)
	if err != nil {
		h.abortSystem(c, "wechat notify query order fail",
			zap.String("order_no", payload.OrderNo), zap.Error(err))
		return
	}
	if order == nil {
		h.logger.Warn("wechat notify order not found", zap.String("order_no", payload.OrderNo))
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}

	// 金额校验：回调实付金额必须与订单金额一致
	if payload.PaidAmount != order.Amount {
		h.logger.Error("wechat notify amount mismatch",
			zap.String("order_no", payload.OrderNo),
			zap.Int64("expect", order.Amount),
			zap.Int64("got", payload.PaidAmount))
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}

	// 幂等入账：条件更新，仅首次（CREATED→PAID）返回 true
	updated, err := h.order.MarkPaid(c.Request.Context(), payload.OrderNo,
		payload.PaidAmount, vo.ChannelWeChat, payload.ChannelTradeNo)
	if err != nil {
		h.abortSystem(c, "wechat notify mark paid fail",
			zap.String("order_no", payload.OrderNo), zap.Error(err))
		return
	}
	if !updated {
		h.logger.Info("wechat notify duplicate (order already paid)", zap.String("order_no", payload.OrderNo))
		h.ok(c)
		return
	}

	// 首次入账：通道在途资金户 → 商户备付金
	if err := h.credit(c.Request.Context(), order.MerchantID, order.OrderNo, payload.PaidAmount); err != nil {
		h.abortSystem(c, "wechat notify credit fail",
			zap.String("order_no", payload.OrderNo), zap.Error(err))
		return
	}

	// 业务处理成功后再落幂等键
	h.saveIdem(c.Request.Context(), idemScopeNotify, payload.NotifyID)

	h.logger.Info("wechat notify processed",
		zap.String("order_no", payload.OrderNo),
		zap.String("channel_trade_no", payload.ChannelTradeNo),
		zap.Bool("first", updated))
	h.ok(c)
}

// credit 回调入账：给商户备付金钱包入账（通道在途资金户流出）。
func (h *Handler) credit(ctx context.Context, merchantID uint64, orderNo string, amount int64) error {
	return h.ledgerSvc.CreditFromSettlement(ctx, &ledger.CreditFromSettlementRequest{
		SettlementWalletID: h.settlementWalletID,
		ToEntityID:         merchantID,
		ToEntityType:       vo.EntityMerchant,
		Amount:             amount,
		BizType:            "PAYMENT",
		BizID:              orderNo,
		TraceID:            "",
	})
}

// HandleWechatRefund POST /v1/notify/wechat/refund。
// 本轮仅验签 + 解密 + log，不做钱包反向入账（完整退款链路放 P2 退款服务）。
func (h *Handler) HandleWechatRefund(c *gin.Context) {
	adapter := h.resolveWechat(c)
	if adapter == nil {
		h.logger.Warn("wechat refund notify received but channel disabled")
		c.AbortWithStatus(http.StatusServiceUnavailable)
		return
	}

	raw, err := io.ReadAll(c.Request.Body)
	if err != nil {
		h.logger.Warn("read wechat refund body", zap.Error(err))
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}

	headers := extractNotifyHeaders(c)
	plain, err := adapter.VerifyAndDecrypt(c.Request.Context(), raw, headers)
	if err != nil {
		h.logger.Warn("wechat refund verify fail",
			zap.String("trace_id", c.GetString("trace_id")), zap.Error(err))
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}

	// 提取信封唯一 id 用于幂等
	var env struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(raw, &env)
	if h.idemHit(c.Request.Context(), idemScopeRefund, env.ID) {
		h.ok(c)
		return
	}

	var payload struct {
		OutTradeNo    string `json:"out_trade_no"`
		OutRefundNo   string `json:"out_refund_no"`
		TransactionID string `json:"transaction_id"`
		RefundID      string `json:"refund_id"`
		Status        string `json:"status"` // SUCCESS / CLOSED / ABNORMAL
		Amount        struct {
			Refund int `json:"refund"`
			Total  int `json:"total"`
		} `json:"amount"`
	}
	if err := json.Unmarshal(plain, &payload); err != nil {
		h.logger.Warn("wechat refund parse fail", zap.Error(err))
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}

	h.logger.Info("wechat refund notify",
		zap.String("out_trade_no", payload.OutTradeNo),
		zap.String("out_refund_no", payload.OutRefundNo),
		zap.String("status", payload.Status),
	)

	h.saveIdem(c.Request.Context(), idemScopeRefund, env.ID)
	h.ok(c)
}

// idemHit 幂等命中返回 true（同一 key 已处理过）。
func (h *Handler) idemHit(ctx context.Context, scope, key string) bool {
	if h.idem == nil || key == "" {
		return false
	}
	existing, err := h.idem.Get(ctx, scope, key)
	if err != nil {
		h.logger.Warn("idem get fail", zap.String("scope", scope), zap.String("key", key), zap.Error(err))
		return false
	}
	return existing != nil
}

// saveIdem 保存幂等记录（失败仅告警，不阻断回调成功响应）。
func (h *Handler) saveIdem(ctx context.Context, scope, key string) {
	if h.idem == nil || key == "" {
		return
	}
	if err := h.idem.Save(ctx, &idem.Record{
		Scope:    scope,
		Key:      key,
		Status:   1,
		ExpireAt: time.Now().Add(idemExpireDays),
	}); err != nil {
		h.logger.Warn("idem save fail", zap.String("scope", scope), zap.String("key", key), zap.Error(err))
	}
}

// extractNotifyHeaders 提取微信回调验签头。
func extractNotifyHeaders(c *gin.Context) map[string]string {
	return map[string]string{
		"Wechatpay-Signature": c.GetHeader("Wechatpay-Signature"),
		"Wechatpay-Timestamp": c.GetHeader("Wechatpay-Timestamp"),
		"Wechatpay-Nonce":     c.GetHeader("Wechatpay-Nonce"),
		"Wechatpay-Serial":    c.GetHeader("Wechatpay-Serial"),
	}
}

// ok 返回微信要求的成功响应。
func (h *Handler) ok(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"code": "SUCCESS", "message": "成功"})
}

// abortSystem 记录系统级（5xx）错误并返回 500 让微信重试。
// 日志带 retry 字段便于 grep 排查。
func (h *Handler) abortSystem(c *gin.Context, msg string, fields ...zap.Field) {
	fields = append(fields,
		zap.String("trace_id", c.GetString("trace_id")),
		zap.String("retry", policyName(retryYes)),
	)
	h.logger.Error(msg, fields...)
	c.AbortWithStatus(http.StatusInternalServerError)
}

// policyName 返回重试策略的可读名。
func policyName(p retryPolicy) string {
	if p == retryYes {
		return "yes"
	}
	return "no"
}
