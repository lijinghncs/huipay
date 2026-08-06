// 包 obs 提供日志、链路追踪等可观测性能力。
package obs

import (
	"context"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const TraceIDKey = "trace_id"

// NewZapLogger 构造结构化 JSON 日志器。
func NewZapLogger(level string) *zap.Logger {
	lvl := zap.InfoLevel
	_ = lvl.UnmarshalText([]byte(level))

	encCfg := zap.NewProductionEncoderConfig()
	encCfg.TimeKey = "ts"
	encCfg.EncodeTime = zapcore.ISO8601TimeEncoder
	encCfg.EncodeLevel = zapcore.LowercaseLevelEncoder

	core := zapcore.NewCore(
		zapcore.NewJSONEncoder(encCfg),
		zapcore.AddSync(os.Stdout),
		lvl,
	)
	return zap.New(core, zap.AddCaller(), zap.AddStacktrace(zap.ErrorLevel))
}

// GinTrace 注入 trace_id 到上下文与响应头。
func GinTrace() gin.HandlerFunc {
	return func(c *gin.Context) {
		tid := c.GetHeader("X-Trace-Id")
		if tid == "" {
			tid = uuid.NewString()
		}
		c.Set(TraceIDKey, tid)
		c.Writer.Header().Set("X-Trace-Id", tid)
		c.Next()
	}
}

// GinAccessLog 输出访问日志。
func GinAccessLog(logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		logger.Info("http_access",
			zap.String("trace_id", c.GetString(TraceIDKey)),
			zap.String("method", c.Request.Method),
			zap.String("path", c.Request.URL.Path),
			zap.Int("status", c.Writer.Status()),
			zap.Duration("cost", time.Since(start)),
			zap.String("client_ip", c.ClientIP()),
		)
	}
}

// GinRecovery 捕获 panic 并返回 500。
func GinRecovery(logger *zap.Logger) gin.HandlerFunc {
	return gin.CustomRecoveryWithWriter(nil, func(c *gin.Context, err any) {
		logger.Error("panic_recovered",
			zap.Any("err", err),
			zap.String("path", c.Request.URL.Path),
		)
		c.AbortWithStatus(500)
	})
}

// TraceIDFromContext 安全获取 trace_id。
func TraceIDFromContext(ctx context.Context) string {
	if v := ctx.Value(TraceIDKey); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}