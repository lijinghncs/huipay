package wechat

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/huipay/huipay-backend/infra/errs"
)

// TestMapErr 覆盖核心 12 个微信错误码映射 + 未命中兜底。
func TestMapErr(t *testing.T) {
	cases := []struct {
		name string
		code string
		want string
	}{
		{"ORDERPAID", "ORDERPAID", errs.CodeInvalidParams},
		{"ORDERCLOSED", "ORDERCLOSED", errs.CodeInvalidParams},
		{"ORDERNOTEXIST", "ORDERNOTEXIST", errs.CodeInvalidParams},
		{"INVALID_REQUEST", "INVALID_REQUEST", errs.CodeInvalidParams},
		{"PARAM_ERROR", "PARAM_ERROR", errs.CodeInvalidParams},
		{"APPID_MCHID_NOT_MATCH", "APPID_MCHID_NOT_MATCH", errs.CodeChannelUnavailable},
		{"SYSTEMERROR", "SYSTEMERROR", errs.CodeInternalError},
		{"BANKERROR", "BANKERROR", errs.CodeChannelUnavailable},
		{"USERPAYING", "USERPAYING", errs.CodeChannelUnavailable},
		{"FREQUENCY_LIMITED", "FREQUENCY_LIMITED", errs.CodeInternalError},
		{"RATE_LIMITED", "RATE_LIMITED", errs.CodeInternalError},
		{"空码兜底", "", errs.CodeInternalError},
		{"未命中兜底", "UNKNOWN_CODE_XYZ", errs.CodeInternalError},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			be := MapErr(http.StatusBadRequest, apiError{Code: c.code, Message: "boom"})
			if be.Code != c.want {
				t.Fatalf("MapErr(%q).Code = %q, want %q", c.code, be.Code, c.want)
			}
			if be.HTTPStatus != http.StatusBadRequest {
				t.Fatalf("HTTPStatus = %d, want %d", be.HTTPStatus, http.StatusBadRequest)
			}
			if be.Message != "boom" {
				t.Fatalf("Message = %q, want %q", be.Message, "boom")
			}
		})
	}
}

// TestMapErrDefaultMessage 消息为空时兜底为固定文案。
func TestMapErrDefaultMessage(t *testing.T) {
	be := MapErr(http.StatusBadRequest, apiError{Code: "ORDERPAID"})
	if be.Message != "wechat API error" {
		t.Fatalf("Message = %q, want %q", be.Message, "wechat API error")
	}
}

// TestMapErrWrapAsync 验证 doJSON 返回的错误可被 errors.As 提取业务码。
func TestMapErrWrapAsync(t *testing.T) {
	bizErr := MapErr(http.StatusBadRequest, apiError{Code: "BANKERROR", Message: "bank down"})
	combined := fmt.Errorf("%w (raw: 502 %s)", bizErr, "/v3/pay/transactions/native")

	var be *errs.BizError
	if !errors.As(combined, &be) {
		t.Fatalf("errors.As failed to extract BizError")
	}
	if be.Code != errs.CodeChannelUnavailable {
		t.Fatalf("Code = %q, want %q", be.Code, errs.CodeChannelUnavailable)
	}
}