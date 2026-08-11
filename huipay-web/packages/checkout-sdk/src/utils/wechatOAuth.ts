// 微信内 OAuth 授权相关工具（公众号网页授权 snsapi_base，用于 JSAPI 支付获取 openid）。

/** 是否运行在微信内置浏览器中。 */
export function isWeixinBrowser(): boolean {
  return /MicroMessenger/i.test(navigator.userAgent);
}

/** 从当前 URL 读取 openid（授权回调后由后端追加到 query）。 */
export function readOpenId(): string {
  return new URLSearchParams(window.location.search).get('openid') ?? '';
}

/** 构造后端微信 OAuth 授权跳转地址。 */
export function buildOAuthAuthorizeUrl(apiBase: string, redirectUri: string): string {
  return `${apiBase}/v1/oauth/wechat/authorize?redirect_uri=${encodeURIComponent(redirectUri)}`;
}

/**
 * 在微信内且缺 openid 时，发起微信网页授权跳转。
 * @returns 是否已发起跳转（true 表示页面即将离开，调用方应停止后续渲染）
 */
export function ensureWechatOpenId(apiBase: string): boolean {
  if (isWeixinBrowser() && !readOpenId()) {
    window.location.href = buildOAuthAuthorizeUrl(apiBase, window.location.href);
    return true;
  }
  return false;
}