// 包 oauth 提供微信网页授权（snsapi_base）接口，用于收银台 H5 获取用户 openid。
package oauth

import (
	"encoding/base64"
	"net/http"
	"net/url"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/huipay/huipay-backend/infra/errs"
	"github.com/huipay/huipay-backend/internal/payment/channel/wechat"
)

// Handler 微信 OAuth Handler。
type Handler struct {
	oauth  *wechat.OAuthClient
	cbBase string // 后端回调基础地址（微信回调需指向后端，避免 secret 泄露）
	logger *zap.Logger
}

// New 构造 Handler。
// cbBase 为后端回调前缀（通常取微信 NotifyBaseURL），最终回调地址为 cbBase + "/v1/oauth/wechat/callback"。
func New(oauth *wechat.OAuthClient, cbBase string, logger *zap.Logger) *Handler {
	return &Handler{oauth: oauth, cbBase: cbBase, logger: logger}
}

// Authorize GET /v1/oauth/wechat/authorize?redirect_uri=xxx&state=yyy
// redirect_uri：授权成功后要跳回的前端页面地址（带订单参数）。
// 返回 302 到微信授权页；微信回调后端 callback 后，由 callback 再跳回 redirect_uri。
func (h *Handler) Authorize(c *gin.Context) {
	redirect := c.Query("redirect_uri")
	if redirect == "" {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, "redirect_uri required", 400))
		return
	}
	if _, err := url.Parse(redirect); err != nil {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, "invalid redirect_uri", 400))
		return
	}
	// state 透传前端跳回地址（base64url 编码，规避微信 state 长度/字符限制）
	state := base64.RawURLEncoding.EncodeToString([]byte(redirect))
	authURL := h.oauth.AuthorizeURL(h.cbBase+"/v1/oauth/wechat/callback", state)
	c.Redirect(http.StatusFound, authURL)
}

// Callback GET /v1/oauth/wechat/callback?code=xxx&state=yyy
// 用 code 向微信换取 openid，随后 302 跳回前端页面并追加 openid 参数。
func (h *Handler) Callback(c *gin.Context) {
	code := c.Query("code")
	if code == "" {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, "code required", 400))
		return
	}
	openid, err := h.oauth.ExchangeOpenID(c.Request.Context(), code)
	if err != nil {
		h.logger.Error("wechat oauth exchange openid fail", zap.Error(err))
		errs.Fail(c, h.logger, errs.New(errs.CodeChannelUnavailable, "oauth exchange fail", 502))
		return
	}

	rawState, err := base64.RawURLEncoding.DecodeString(c.Query("state"))
	if err != nil || len(rawState) == 0 {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, "invalid state", 400))
		return
	}
	frontURL, err := url.Parse(string(rawState))
	if err != nil {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, "invalid state", 400))
		return
	}
	q := frontURL.Query()
	q.Set("openid", openid)
	frontURL.RawQuery = q.Encode()
	c.Redirect(http.StatusFound, frontURL.String())
}