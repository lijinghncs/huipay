package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// CORS 返回跨域中间件，用于本地开发前后端联调（5173 → 8080）。
// 默认放行任意来源；生产环境应改为白名单配置并收紧请求头。
func CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		// 反射预检声明的请求头，避免客户端新增自定义头（如 Idempotency-Key / X-Trace-Id）时被白名单漏掉
		if reqHeaders := c.GetHeader("Access-Control-Request-Headers"); reqHeaders != "" {
			c.Header("Access-Control-Allow-Headers", reqHeaders)
		} else {
			c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Merchant-Id, X-Request-ID")
		}
		c.Header("Access-Control-Max-Age", "86400")

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}