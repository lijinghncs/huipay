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
	"gopkg.in/natefinch/lumberjack.v2"
)

const TraceIDKey = "trace_id"

// FileLogConfig 日志文件输出配置（落盘 + 轮转）。
type FileLogConfig struct {
	Enabled   bool   // 是否写入文件
	Path      string // 日志文件路径，如 logs/app.log
	MaxSizeMB int    // 单个日志文件最大大小（MB），超过则轮转
	MaxAgeDay int    // 保留天数
}

// NewZapLogger 构造结构化 JSON 日志器。
// 默认输出到标准输出；配置了文件路径时，同时写文件（lumberjack 按大小/天数轮转）。
func NewZapLogger(level string, file FileLogConfig) *zap.Logger {
	lvl := zap.InfoLevel
	_ = lvl.UnmarshalText([]byte(level))

	encCfg := zap.NewProductionEncoderConfig()
	encCfg.TimeKey = "ts"
	encCfg.EncodeTime = zapcore.ISO8601TimeEncoder
	encCfg.EncodeLevel = zapcore.LowercaseLevelEncoder

	encoder := zapcore.NewJSONEncoder(encCfg)
	// 始终输出到标准输出
	cores := []zapcore.Core{
		zapcore.NewCore(encoder, zapcore.AddSync(os.Stdout), lvl),
	}
	// 追加文件输出（按大小/天数轮转）
	if file.Enabled && file.Path != "" {
		w := &lumberjack.Logger{
			Filename: file.Path,
			MaxSize:  file.MaxSizeMB,
			MaxAge:   file.MaxAgeDay,
			Compress: true,
			LocalTime: true,
		}
		cores = append(cores, zapcore.NewCore(encoder, zapcore.AddSync(w), lvl))
	}
	core := zapcore.NewTee(cores...)
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