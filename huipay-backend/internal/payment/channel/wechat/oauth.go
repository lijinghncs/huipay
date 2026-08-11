// 微信公众平台网页授权（OAuth2 snsapi_base）客户端。
// 用于在微信内打开的收银台 H5 页面获取用户 openid，供 JSAPI 支付使用。
package wechat

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/huipay/huipay-backend/infra/config"
)

const (
	// oauthAuthorizeURL 微信网页授权跳转地址。
	oauthAuthorizeURL = "https://open.weixin.qq.com/connect/oauth2/authorize"
	// oauthTokenURL 用 code 换取 openid/access_token 的接口。
	oauthTokenURL = "https://api.weixin.qq.com/sns/oauth2/access_token"
)

// OAuthClient 公众号网页授权客户端。
type OAuthClient struct {
	appID     string
	appSecret string
	http      *http.Client
}

// NewOAuthClient 构造 OAuth 客户端。
func NewOAuthClient(cfg config.WeChatConfig) *OAuthClient {
	return &OAuthClient{
		appID:     cfg.AppID,
		appSecret: cfg.AppSecret,
		http:      &http.Client{Timeout: 10 * time.Second},
	}
}

// AuthorizeURL 生成微信网页授权跳转链接（scope=snsapi_base 静默授权）。
// redirectURI 必填：微信授权成功后回调地址（须为微信后台配置的域名，通常指向后端 callback）。
// state 自定义透传参数，用于回调后跳回前端页面。
func (o *OAuthClient) AuthorizeURL(redirectURI, state string) string {
	q := url.Values{}
	q.Set("appid", o.appID)
	q.Set("redirect_uri", redirectURI)
	q.Set("response_type", "code")
	q.Set("scope", "snsapi_base")
	q.Set("state", state)
	return oauthAuthorizeURL + "?" + q.Encode() + "#wechat_redirect"
}

// ExchangeOpenID 用授权 code 换取 openid（snsapi_base 仅返回 openid，无用户资料）。
func (o *OAuthClient) ExchangeOpenID(ctx context.Context, code string) (string, error) {
	q := url.Values{}
	q.Set("appid", o.appID)
	q.Set("secret", o.appSecret)
	q.Set("code", code)
	q.Set("grant_type", "authorization_code")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, oauthTokenURL+"?"+q.Encode(), nil)
	if err != nil {
		return "", fmt.Errorf("wechat oauth: build request: %w", err)
	}
	resp, err := o.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("wechat oauth: exchange openid: %w", err)
	}
	defer resp.Body.Close()

	var body struct {
		Errcode int    `json:"errcode"`
		Errmsg  string `json:"errmsg"`
		OpenID  string `json:"openid"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", fmt.Errorf("wechat oauth: decode response: %w", err)
	}
	if body.Errcode != 0 {
		return "", fmt.Errorf("wechat oauth: errcode=%d errmsg=%s", body.Errcode, body.Errmsg)
	}
	if body.OpenID == "" {
		return "", fmt.Errorf("wechat oauth: empty openid")
	}
	return body.OpenID, nil
}