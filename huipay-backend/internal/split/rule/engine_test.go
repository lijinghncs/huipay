package rule

import (
	"testing"
)

func TestParseConditions(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    Condition
		wantErr bool
	}{
		{"empty", "", Condition{}, false},
		{"channel only", `{"channel":"wechat"}`, Condition{Channel: "wechat"}, false},
		{"store_ids", `{"store_ids":[1,2]}`, Condition{StoreIDs: []uint64{1, 2}}, false},
		{"invalid json", `{invalid`, Condition{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseConditions(tt.raw)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseConditions() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got.Channel != tt.want.Channel {
				t.Errorf("ParseConditions() Channel = %v, want %v", got.Channel, tt.want.Channel)
			}
		})
	}
}

func TestParseAllocations(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		wantLen int
		wantErr bool
	}{
		{"empty", "", 0, false},
		{"single", `[{"receiver_entity_id":1,"receiver_type":"STORE","ratio_bps":5000}]`, 1, false},
		{"multiple", `[{"receiver_entity_id":1,"receiver_type":"STORE","ratio_bps":3000},{"receiver_entity_id":2,"receiver_type":"STORE","ratio_bps":7000}]`, 2, false},
		{"invalid", `{invalid`, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseAllocations(tt.raw)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseAllocations() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if len(got) != tt.wantLen {
				t.Errorf("ParseAllocations() len = %d, want %d", len(got), tt.wantLen)
			}
		})
	}
}

func TestRule_Match(t *testing.T) {
	r := &Rule{
		MerchantID: 100,
		Priority:   10,
		Conditions: Condition{
			Channel:  "wechat",
			StoreIDs: []uint64{1, 2, 3},
		},
	}

	tests := []struct {
		name string
		ctx  MatchContext
		want bool
	}{
		{"full match", MatchContext{MerchantID: 100, Channel: "wechat", StoreID: 1}, true},
		{"wrong merchant", MatchContext{MerchantID: 200, Channel: "wechat", StoreID: 1}, false},
		{"wrong channel", MatchContext{MerchantID: 100, Channel: "alipay", StoreID: 1}, false},
		{"wrong store", MatchContext{MerchantID: 100, Channel: "wechat", StoreID: 99}, false},
		{"merchant 0 match all", MatchContext{MerchantID: 999, Channel: "wechat", StoreID: 1}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := r.Match(tt.ctx); got != tt.want {
				t.Errorf("Rule.Match() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRule_Match_NoConditions(t *testing.T) {
	// 无条件规则应匹配任何商户
	r := &Rule{
		MerchantID: 100,
		Priority:   1,
		Conditions: Condition{},
	}
	if !r.Match(MatchContext{MerchantID: 100}) {
		t.Error("Rule without conditions should match")
	}
}

func TestEngine_Resolve(t *testing.T) {
	rules := []Rule{
		{RuleCode: "low", MerchantID: 100, Priority: 1, Conditions: Condition{Channel: "wechat"}},
		{RuleCode: "high", MerchantID: 100, Priority: 10, Conditions: Condition{Channel: "wechat"}},
		{RuleCode: "mid", MerchantID: 100, Priority: 5, Conditions: Condition{Channel: "alipay"}},
	}

	e := &Engine{}

	tests := []struct {
		name string
		ctx  MatchContext
		want string
	}{
		{"wechat returns highest priority", MatchContext{MerchantID: 100, Channel: "wechat"}, "high"},
		{"alipay returns mid", MatchContext{MerchantID: 100, Channel: "alipay"}, "mid"},
		{"no match returns nil", MatchContext{MerchantID: 200, Channel: "wechat"}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := e.Resolve(rules, tt.ctx)
			if tt.want == "" {
				if got != nil {
					t.Errorf("Resolve() = %v, want nil", got.RuleCode)
				}
				return
			}
			if got == nil {
				t.Fatal("Resolve() = nil, want non-nil")
			}
			if got.RuleCode != tt.want {
				t.Errorf("Resolve() = %v, want %v", got.RuleCode, tt.want)
			}
		})
	}
}

func TestEngine_Resolve_EmptyRules(t *testing.T) {
	e := &Engine{}
	if got := e.Resolve(nil, MatchContext{MerchantID: 100}); got != nil {
		t.Error("Resolve with nil rules should return nil")
	}
	if got := e.Resolve([]Rule{}, MatchContext{MerchantID: 100}); got != nil {
		t.Error("Resolve with empty rules should return nil")
	}
}