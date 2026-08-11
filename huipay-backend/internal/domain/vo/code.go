// 包 vo 定义值对象。
package vo

// CodeStatus 收款码牌状态。
type CodeStatus int

const (
	CodeDisabled CodeStatus = 0 // 停用
	CodeActive   CodeStatus = 1 // 启用
)