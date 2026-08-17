// 包 middleware 提供跨业务中间件。
package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/huipay/huipay-backend/infra/auth"
	"github.com/huipay/huipay-backend/infra/errs"
)

// NewAdminAuth 管理后台强鉴权中间件：校验 Authorization: Bearer <admin token>。
// 仅对 /v1/admin/* 路径强制校验；其余路径直接放行（由商户中间件负责）。
// 校验通过后注入 admin_id / admin_role 到 gin 上下文。
// secret 为空时直接放行（开发模式，未配置签名密钥不拦截，避免阻塞本地联调）。
func NewAdminAuth(secret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 登录接口无需鉴权
		if c.Request.URL.Path == "/v1/admin/login" {
			c.Next()
			return
		}
		if !strings.HasPrefix(c.Request.URL.Path, "/v1/admin/") {
			c.Next()
			return
		}
		if secret == "" {
			c.Next()
			return
		}
		h := c.GetHeader("Authorization")
		if !strings.HasPrefix(h, "Bearer ") {
			abortUnauthorized(c)
			return
		}
		claims, err := auth.Verify(secret, strings.TrimPrefix(h, "Bearer "))
		if err != nil || claims.Role != "admin" {
			abortUnauthorized(c)
			return
		}
		c.Set("admin_id", claims.MerchantID)
		c.Set("admin_role", claims.Role)
		c.Next()
	}
}

// abortUnauthorized 401 中止请求。
func abortUnauthorized(c *gin.Context) {
	c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
		"code":    errs.CodeUnauthorized,
		"message": "未授权或登录已过期",
	})
}