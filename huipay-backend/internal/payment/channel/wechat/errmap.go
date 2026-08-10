// 包 wechat 微信支付 API 错误码到业务错误码的映射。
package wechat

import (
	"github.com/huipay/huipay-backend/infra/errs"
)

// ErrCodeMap 微信 API 错误码 → 业务错误映射表（子集覆盖核心 12 个码）。
var ErrCodeMap = map[string]string{
	// 用户侧
	"ORDERPAID":     errs.CodeInvalidParams,
	"ORDERCLOSED":   errs.CodeInvalidParams,
	"ORDERNOTEXIST": errs.CodeInvalidParams,
	// 参数类
	"INVALID_REQUEST":       errs.CodeInvalidParams,
	"PARAM_ERROR":           errs.CodeInvalidParams,
	"APPID_MCHID_NOT_MATCH": errs.CodeChannelUnavailable,
	// 系统类
	"SYSTEMERROR": errs.CodeInternalError,
	"BANKERROR":   errs.CodeChannelUnavailable,
	"USERPAYING":  errs.CodeChannelUnavailable,
	// 频率类
	"FREQUENCY_LIMITED": errs.CodeInternalError,
	"RATE_LIMITED":      errs.CodeInternalError,
	// 兜底
	"": errs.CodeInternalError,
}

// MapErr 将微信 API 错误码转换为 BizError。
// 命中映射表 → 返回对应业务码；未命中 → CodeInternalError（系统异常，触发重试）。
func MapErr(httpStatus int, ae apiError) *errs.BizError {
	code, ok := ErrCodeMap[ae.Code]
	if !ok {
		code = errs.CodeInternalError
	}
	msg := ae.Message
	if msg == "" {
		msg = "wechat API error"
	}
	return errs.New(code, msg, httpStatus)
}