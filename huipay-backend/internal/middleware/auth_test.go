package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/huipay/huipay-backend/infra/auth"
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

// TestNewMerchantAuthBearer 验证 Bearer token 优先于 X-Merchant-Id。
func TestNewMerchantAuthBearer(t *testing.T) {
	gin.SetMode(gin.TestMode)
	token, err := auth.Sign("test-secret", 42)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Merchant-Id", "999") // 应被 token 覆盖
	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	ctx.Request = req

	NewMerchantAuth("test-secret", true)(ctx)
	if got := ctx.GetUint64("merchant_id"); got != 42 {
		t.Fatalf("merchant_id = %d, want 42 (bearer wins)", got)
	}
}

// TestNewMerchantAuthNoTrustHeader 验证 trustHeader=false 时不信任明文头。
func TestNewMerchantAuthNoTrustHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Merchant-Id", "42")
	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	ctx.Request = req

	NewMerchantAuth("test-secret", false)(ctx)
	if _, ok := ctx.Get("merchant_id"); ok {
		t.Fatal("merchant_id should not be set when trustHeader=false")
	}
}

// TestNewMerchantAuthInvalidBearer 无效 token 时按 trustHeader 回退。
func TestNewMerchantAuthInvalidBearer(t *testing.T) {
	gin.SetMode(gin.TestMode)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer invalid.token.here")
	req.Header.Set("X-Merchant-Id", "7")
	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	ctx.Request = req

	NewMerchantAuth("test-secret", true)(ctx)
	if got := ctx.GetUint64("merchant_id"); got != 7 {
		t.Fatalf("merchant_id = %d, want 7 (fallback header)", got)
	}
}
