// 包 middleware 提供跨业务中间件。
package middleware

import (
	"strconv"

	"github.com/gin-gonic/gin"
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