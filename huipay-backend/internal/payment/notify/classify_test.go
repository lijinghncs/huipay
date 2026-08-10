package notify

import (
	"errors"
	"net/http"
	"testing"

	"github.com/huipay/huipay-backend/infra/errs"
)

// TestClassifyError 覆盖 5xx / 业务级 4xx / 含高状态码 BizError 三类分级。
func TestClassifyError(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		status int
		want   retryPolicy
	}{
		{"5xx 系统错", errors.New("boom"), http.StatusInternalServerError, retryYes},
		{"4xx 业务错", errors.New("boom"), http.StatusBadRequest, retryNo},
		{"BizError HTTP>=500", errs.New(errs.CodeInternalError, "db fail", http.StatusInternalServerError), http.StatusBadRequest, retryYes},
		{"BizError HTTP<500", errs.New(errs.CodeInvalidParams, "bad", http.StatusBadRequest), http.StatusBadRequest, retryNo},
		{"普通错误 4xx", errors.New("boom"), http.StatusBadRequest, retryNo},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := classifyError(c.err, c.status); got != c.want {
				t.Fatalf("classifyError() = %v, want %v", got, c.want)
			}
		})
	}
}

// TestPolicyName 可读命名的互斥性。
func TestPolicyName(t *testing.T) {
	if policyName(retryYes) != "yes" || policyName(retryNo) != "no" {
		t.Fatalf("policyName mismatch: yes=%q no=%q", policyName(retryYes), policyName(retryNo))
	}
}