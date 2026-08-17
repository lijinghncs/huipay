package repository

import "encoding/json"

// marshalDetail JSON 序列化 detail 字段；失败返回空字符串（不让审计写入失败阻断业务）。
func marshalDetail(v any) (string, error) {
	if v == nil {
		return "", nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(b), nil
}