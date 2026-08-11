package service

import (
	"reflect"
	"testing"

	"github.com/huipay/huipay-backend/internal/domain/entity"
)

// 期望被 AES 加密的敏感字段 JSON tag（与 wechat_config.go 注释标记"敏感"一致）。
// 若新增敏感字段，需同步登记到 entity.SensitiveFields 与下列列表，否则测试失败。
var wantSensitiveTags = []string{"app_secret", "api_v3_key", "merchant_private_key", "platform_public_key"}

// TestSensitiveFieldsDeclared 断言所有敏感字段均已登记到 entity.SensitiveFields，
// 防止新增敏感字段漏配导致明文入库。
func TestSensitiveFieldsDeclared(t *testing.T) {
	declared := map[string]bool{}
	for _, f := range entity.SensitiveFields {
		declared[f] = true
	}
	for _, tag := range wantSensitiveTags {
		if !declared[tag] {
			t.Errorf("sensitive field %q not declared in entity.SensitiveFields", tag)
		}
	}
}

// TestSensitiveFieldsJSONTagsValid 断言 SensitiveFields 中每个 tag 都对应
// WechatConfig 的一个 string 字段（加密装配反射可命中），避免装配静默漏加密。
func TestSensitiveFieldsJSONTagsValid(t *testing.T) {
	rt := reflect.TypeOf(entity.WechatConfig{})
	fields := map[string]bool{}
	for i := 0; i < rt.NumField(); i++ {
		f := rt.Field(i)
		if f.Type.Kind() == reflect.String {
			fields[f.Tag.Get("json")] = true
		}
	}
	for _, tag := range entity.SensitiveFields {
		if !fields[tag] {
			t.Errorf("entity.SensitiveFields contains %q which is not a string JSON field of WechatConfig", tag)
		}
	}
}