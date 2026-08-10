package wechat

import "testing"

// TestPathCategory 覆盖 pathCategory 的 9 个分类分支。
func TestPathCategory(t *testing.T) {
	cases := []struct {
		name string
		path string
		want string
	}{
		{"native", "/v3/pay/transactions/native", "prepay_native"},
		{"h5", "/v3/pay/transactions/h5", "prepay_h5"},
		{"jsapi", "/v3/pay/transactions/jsapi", "prepay_jsapi"},
		{"query", "/v3/pay/transactions/out-trade-no/HP1?mchid=1001", "query"},
		{"close", "/v3/pay/transactions/out-trade-no/HP1/close", "close"},
		{"refund", "/v3/refund/domestic/refunds", "refund"},
		{"certificates", "/v3/certificates", "certificates"},
		{"bill_download", "/v3/billdownload/tradebill?bill_date=2026-08-10", "bill_download"},
		{"other", "/v3/unknown/path", "other"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := pathCategory(c.path); got != c.want {
				t.Fatalf("pathCategory(%q) = %q, want %q", c.path, got, c.want)
			}
		})
	}
}