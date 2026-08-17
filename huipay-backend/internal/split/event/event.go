// Package event 定义分账域领域事件与事件总线。
//
// P3 引入 outbox + 事件总线，实现通道成功 / 账单审批的链路可视化、跨域解耦。
// 事件写入 t_outbox_event 表，由后台 Worker 轮询投递到内存总线，各 Handler 消费。
package event

import (
	"encoding/json"
	"time"
)

// EventType 事件类型常量。
const (
	TypeSplitOrderExecuted  = "SPLIT_ORDER_EXECUTED"  // 分账执行完成（终态）
	TypeSplitBillApproved   = "SPLIT_BILL_APPROVED"   // 周期分账账单审批通过
	TypeSplitBillRejected   = "SPLIT_BILL_REJECTED"   // 周期分账账单驳回
	TypeReconcileDiffResolved = "RECONCILE_DIFF_RESOLVED" // 对账差异核销
)

// AggregateType 聚合类型常量。
const (
	AggregateSplitOrder = "SPLIT_ORDER"
	AggregateSplitBill  = "SPLIT_BILL"
	AggregateDiff       = "RECONCILE_DIFF"
)

// DomainEvent 领域事件（与 t_outbox_event 表结构对应）。
type DomainEvent struct {
	ID            string          `json:"id"`
	AggregateType string          `json:"aggregate_type"`
	AggregateID   string          `json:"aggregate_id"`
	EventType     string          `json:"event_type"`
	Payload       json.RawMessage `json:"payload"`
	Timestamp     time.Time       `json:"timestamp"`
}

// SplitOrderExecutedPayload 分账执行完成载荷。
type SplitOrderExecutedPayload struct {
	OrderNo      string `json:"order_no"`
	MerchantID   uint64 `json:"merchant_id"`
	Status       string `json:"status"`
	ReceiverCount int   `json:"receiver_count"`
	SuccessCount  int   `json:"success_count"`
	TotalAmount  int64  `json:"total_amount"`
	Degraded     int    `json:"degraded"`
	LastError    string `json:"last_error,omitempty"`
}

// SplitBillApprovedPayload 账单审批通过载荷。
type SplitBillApprovedPayload struct {
	BillID     uint64 `json:"bill_id"`
	BatchNo    string `json:"batch_no"`
	MerchantID uint64 `json:"merchant_id"`
	RuleCode   string `json:"rule_code"`
	TotalAmount int64 `json:"total_amount"`
}

// SplitBillRejectedPayload 账单驳回载荷。
type SplitBillRejectedPayload struct {
	BillID     uint64 `json:"bill_id"`
	BatchNo    string `json:"batch_no"`
	MerchantID uint64 `json:"merchant_id"`
	Reason     string `json:"reason,omitempty"`
}

// ReconcileDiffResolvedPayload 对账差异核销载荷。
type ReconcileDiffResolvedPayload struct {
	DiffID     uint64 `json:"diff_id"`
	BatchNo    string `json:"batch_no"`
	MerchantID uint64 `json:"merchant_id"`
	DiffType   string `json:"diff_type"`
}