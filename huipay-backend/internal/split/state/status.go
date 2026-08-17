// Package state 集中定义分账订单级状态机——状态常量、语义判断与合法转移。
// 所有包统一通过 state.Status 判断状态，不再散落裸字符串比较。
package state

// Status 订单级分账状态。
type Status string

const (
	Pending    Status = "PENDING"    // 待处理
	Processing Status = "PROCESSING" // 处理中
	Success    Status = "SUCCESS"    // 全部成功
	Partial    Status = "PARTIAL"    // 部分成功
	Failed     Status = "FAILED"     // 全部失败
	Suspended  Status = "SUSPENDED"  // 悬挂（处理中超时）
	Dead       Status = "DEAD"       // 重试耗尽
	Resolved   Status = "RESOLVED"   // 人工核销（线下处理完毕，差错闭环）
)

// String 返回状态字符串。
func (s Status) String() string { return string(s) }

// IsTerminal 是否终态——到达后不再自动推进。
func (s Status) IsTerminal() bool {
	return s == Success || s == Dead || s == Resolved
}

// IsClaimable 是否可被补偿调度认领重新处理。
func (s Status) IsClaimable() bool {
	return s == Failed || s == Partial || s == Suspended
}

// IsException 是否属于异常状态集合（差错中心展示）。
func (s Status) IsException() bool {
	return s == Failed || s == Partial || s == Suspended || s == Dead || s == Resolved
}

// IsError 是否属于业务错误状态（需要告警或人工介入）。
func (s Status) IsError() bool {
	return s == Failed || s == Dead || s == Suspended
}

// ExceptionStatuses 差错中心聚合的异常状态集合。
var ExceptionStatuses = []Status{
	Failed, Partial, Suspended, Dead, Resolved,
}

// CanTransition 返回从 from 到 to 是否为合法转移。
// 注意：本函数只校验转移合法性，不负责持久化。
func CanTransition(from, to Status) bool {
	key := transitionKey{from, to}
	_, ok := validTransitions[key]
	return ok
}

// transitionKey 转移矩阵键。
type transitionKey struct{ from, to Status }

// validTransitions 合法转移矩阵。
var validTransitions = map[transitionKey]bool{
	// 初始
	{Pending, Processing}: true,

	// 执行结果
	{Processing, Success}:   true,
	{Processing, Partial}:   true,
	{Processing, Failed}:    true,
	{Processing, Suspended}: true,

	// 补偿调度认领重试
	{Failed, Processing}:    true,
	{Partial, Processing}:   true,
	{Suspended, Processing}: true,

	// 重试耗尽 → 死单
	{Failed, Dead}:    true,
	{Partial, Dead}:   true,
	{Suspended, Dead}: true,

	// 人工核销
	{Dead, Resolved}: true,
}

// SyncToOrderStatus 将分账状态映射为 t_order.split_status 值。
// 仅终态返回映射值；非终态返回空字符串（不写回）。
func SyncToOrderStatus(s Status) string {
	switch s {
	case Success:
		return "SUCCESS"
	case Failed, Dead, Suspended:
		return "FAILED"
	default:
		return ""
	}
}