package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestMerchantIDFromHeader 有效 / 无效 / 缺失三种头。
func TestMerchantIDFromHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cases := []struct {
		name string
		val  string
		want uint64
	}{
		{"有效", "10001", 10001},
		{"无效", "abc", 0},
		{"负数", "-5", 0},
		{"缺失", "", 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if c.val != "" {
				req.Header.Set("X-Merchant-Id", c.val)
			}
			w := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(w)
			ctx.Request = req
			if got := MerchantIDFromHeader(ctx); got != c.want {
				t.Fatalf("MerchantIDFromHeader(%q) = %d, want %d", c.val, got, c.want)
			}
		})
	}
}

// TestMerchantIDMiddleware 中间件仅在头 >0 时注入 merchant_id。
func TestMerchantIDMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// 注入场景
	t.Run("注入", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("X-Merchant-Id", "42")
		w := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(w)
		ctx.Request = req

		var got uint64
		MerchantID()(ctx)
		got = ctx.GetUint64("merchant_id")
		if got != 42 {
			t.Fatalf("merchant_id = %d, want 42", got)
		}
	})

	// 不注入场景（缺失头）
	t.Run("缺失不注入", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		w := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(w)
		ctx.Request = req

		MerchantID()(ctx)
		if got, ok := ctx.Get("merchant_id"); ok && got.(uint64) != 0 {
			t.Fatalf("merchant_id should not be set, got %v", got)
		}
	})
}