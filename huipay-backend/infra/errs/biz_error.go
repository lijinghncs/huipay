// 包 errs 定义业务错误模型与统一响应格式。
package errs

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// 业务错误码常量。
const (
	CodeSuccess             = "0"
	CodeInvalidParams       = "10001"
	CodeUnauthorized        = "10002"
	CodeIdempotentConflict  = "10010"
	CodeInsufficientBalance = "20001"
	CodeChannelUnavailable  = "30001"
	CodeSplitRuleNotMatch   = "40001"
	CodeReconcileDiff       = "50001"
	CodeInternalError       = "99999"
)

// BizError 业务错误。
type BizError struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	HTTPStatus int    `json:"-"`
	Cause      error  `json:"-"`
}

func (e *BizError) Error() string {
	if e.Cause != nil {
		return e.Message + ": " + e.Cause.Error()
	}
	return e.Message
}

// New 构造业务错误。
func New(code, msg string, status int) *BizError {
	return &BizError{Code: code, Message: msg, HTTPStatus: status}
}

// Wrap 包装底层错误。
func Wrap(code, msg string, status int, cause error) *BizError {
	return &BizError{Code: code, Message: msg, HTTPStatus: status, Cause: cause}
}

// As 判断错误是否为 BizError。
func As(err error) (*BizError, bool) {
	var be *BizError
	if errors.As(err, &be) {
		return be, true
	}
	return nil, false
}

// Response 统一响应结构。
type Response struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	TraceID string `json:"trace_id,omitempty"`
	Data    any    `json:"data,omitempty"`
}

// OK 写入成功响应。
func OK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, Response{Code: CodeSuccess, Message: "success", Data: data, TraceID: c.GetString("trace_id")})
}

// Fail 写入业务错误响应。
func Fail(c *gin.Context, logger *zap.Logger, bizErr *BizError) {
	if logger != nil {
		logger.Warn("biz_error",
			zap.String("code", bizErr.Code),
			zap.String("msg", bizErr.Message),
			zap.String("trace_id", c.GetString("trace_id")),
		)
	}
	status := bizErr.HTTPStatus
	if status == 0 {
		status = http.StatusOK
	}
	c.JSON(status, Response{Code: bizErr.Code, Message: bizErr.Message, TraceID: c.GetString("trace_id")})
}

// GinErrorHandler 全局错误处理中间件。
func GinErrorHandler(logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		if len(c.Errors) == 0 {
			return
		}
		last := c.Errors.Last().Err
		if be, ok := As(last); ok {
			Fail(c, logger, be)
			return
		}
		Fail(c, logger, New(CodeInternalError, "internal error", http.StatusInternalServerError))
	}
}