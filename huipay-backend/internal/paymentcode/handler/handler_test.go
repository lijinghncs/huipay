// 包 handler 测试。
package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/huipay/huipay-backend/infra/errs"
	"github.com/huipay/huipay-backend/internal/middleware"
	"github.com/huipay/huipay-backend/internal/paymentcode/repository"
	"github.com/huipay/huipay-backend/internal/paymentcode/service"
)

var seq2 uint64

func newMemDSN2() string {
	n := atomic.AddUint64(&seq2, 1)
	return fmt.Sprintf("file:ph%d?mode=memory&cache=shared", n)
}

func newRouter(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(newMemDSN2()), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&repository.PaymentCodeModel{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	svc := service.NewService(repository.NewPaymentCodeRepo(db), zap.NewNop())
	h := New(svc, zap.NewNop())

	r := gin.New()
	r.Use(middleware.MerchantID())
	r.Use(errs.GinErrorHandler(zap.NewNop()))
	r.POST("/v1/merchant/codes", h.Create)
	r.GET("/v1/merchant/codes", h.List)
	r.POST("/v1/merchant/codes/:id/disable", h.Disable)
	return r
}

func doReq(t *testing.T, r *gin.Engine, method, path, body string, merchantID string) *httptest.ResponseRecorder {
	t.Helper()
	var rd *bytes.Reader
	if body == "" {
		rd = bytes.NewReader(nil)
	} else {
		rd = bytes.NewReader([]byte(body))
	}
	req := httptest.NewRequest(method, path, rd)
	req.Header.Set("Content-Type", "application/json")
	if merchantID != "" {
		req.Header.Set("X-Merchant-Id", merchantID)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestCreateHandler(t *testing.T) {
	r := newRouter(t)
	w := doReq(t, r, http.MethodPost, "/v1/merchant/codes", `{"remark":"吧台"}`, "1001")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Code string `json:"code"`
		Data struct {
			CodeID string `json:"code_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Code != "10001" && resp.Data.CodeID == "" {
		t.Fatalf("unexpected response: %s", w.Body.String())
	}
}

func TestCreateHandlerRequiresMerchant(t *testing.T) {
	r := newRouter(t)
	w := doReq(t, r, http.MethodPost, "/v1/merchant/codes", `{"remark":"x"}`, "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	var resp struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Code != "10001" {
		t.Fatalf("want invalid params code, got %s", resp.Code)
	}
}

func TestListHandler(t *testing.T) {
	r := newRouter(t)
	// 先创建两条
	doReq(t, r, http.MethodPost, "/v1/merchant/codes", `{}`, "1001")
	doReq(t, r, http.MethodPost, "/v1/merchant/codes", `{}`, "1001")
	w := doReq(t, r, http.MethodGet, "/v1/merchant/codes?page=1&page_size=10", "", "1001")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	var resp struct {
		Data struct {
			Total int64 `json:"total"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Data.Total != 2 {
		t.Fatalf("total = %d, want 2", resp.Data.Total)
	}
}

func TestDisableHandler(t *testing.T) {
	r := newRouter(t)
	wCreate := doReq(t, r, http.MethodPost, "/v1/merchant/codes", `{}`, "1001")
	var created struct {
		Data struct {
			ID uint64 `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(wCreate.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal create: %v", err)
	}
	// 其它商户停用应失败
	wOther := doReq(t, r, http.MethodPost, fmt.Sprintf("/v1/merchant/codes/%d/disable", created.Data.ID), "", "9999")
	var otherResp struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(wOther.Body.Bytes(), &otherResp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if otherResp.Code != "10001" {
		t.Fatalf("want invalid params for cross-merchant disable, got %s", otherResp.Code)
	}
	// 本商户停用成功
	wOK := doReq(t, r, http.MethodPost, fmt.Sprintf("/v1/merchant/codes/%d/disable", created.Data.ID), "", "1001")
	if wOK.Code != http.StatusOK {
		t.Fatalf("status = %d", wOK.Code)
	}
}