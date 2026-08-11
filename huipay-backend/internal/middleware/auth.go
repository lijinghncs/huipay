// 包 middleware 提供跨业务中间件。
package middleware

import (
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/huipay/huipay-backend/infra/auth"
)

// MerchantIDFromHeader 解析 X-Merchant-Id 请求头为商户号。
// 开发模式信任该头；生产环境应替换为 JWT 鉴权（见 plan P5 OAuth）。
// 缺失或非法时返回 0。
func MerchantIDFromHeader(c *gin.Context) uint64 {
	id, _ := strconv.ParseUint(c.GetHeader("X-Merchant-Id"), 10, 64)
	return id
}

// MerchantID 中间件：从 X-Merchant-Id 头注入 merchant_id 到 gin 上下文。
// 仅当头部存在且 >0 时设置，供 Precreate/Pay/Refund/List 等 handler 覆盖 body 商户号。
func MerchantID() gin.HandlerFunc {
	return func(c *gin.Context) {
		if id := MerchantIDFromHeader(c); id > 0 {
			c.Set("merchant_id", id)
		}
		c.Next()
	}
}

// NewMerchantAuth 构造商户鉴权中间件：
//  1. 优先解析 Authorization: Bearer <token>（auth.Verify，生产推荐）；
//  2. 无有效 token 且 trustHeader=true 时，回退信任 X-Merchant-Id 明文头（仅开发/联调）。
//
// secret 为空时 Bearer 解析直接跳过（等价于纯头信任）。
func NewMerchantAuth(secret string, trustHeader bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		if claims, err := authFromBearer(c, secret); err == nil && claims != nil {
			c.Set("merchant_id", claims.MerchantID)
			c.Next()
			return
		}
		if trustHeader {
			if id := MerchantIDFromHeader(c); id > 0 {
				c.Set("merchant_id", id)
			}
		}
		c.Next()
	}
}

// authFromBearer 解析 Bearer token；无 token 或 secret 为空返回 nil。
func authFromBearer(c *gin.Context, secret string) (*auth.Claims, error) {
	h := c.GetHeader("Authorization")
	if secret == "" || !strings.HasPrefix(h, "Bearer ") {
		return nil, nil
	}
	return auth.Verify(secret, strings.TrimPrefix(h, "Bearer "))
}
