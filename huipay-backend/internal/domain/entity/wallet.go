// 包 entity 定义领域实体。
package entity

import (
	"time"

	"github.com/huipay/huipay-backend/internal/domain/vo"
)

// Wallet 钱包。
type Wallet struct {
	ID         uint64
	WalletNo   string
	EntityID   uint64
	EntityType vo.EntityType
	Currency   string
	Balance    int64 // 可用余额（分）
	Frozen     int64 // 冻结余额（分）
	PreFrozen  int64 // 预冻结（分账执行中）（分）
	Version    int64 // 乐观锁
	Status     int   // 1启用 0停用
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// JournalEntry 账本流水（不可变）。
type JournalEntry struct {
	ID             string
	WalletID       uint64
	Direction      string // DEBIT / CREDIT
	Amount         int64
	BalanceAfter   int64
	BizType        string // PAYMENT / SPLIT / REFUND / WITHDRAW / ADJUST
	BizID          string
	CounterpartyID *uint64
	IdempotencyKey string
	TraceID        string
	Remark         string
	CreatedAt      time.Time
}

// Order 订单。
type Order struct {
	ID              uint64
	OrderNo         string
	MerchantOrderNo string
	MerchantID      uint64
	ParentOrderNo   string
	OrderType       string
	Amount          int64
	PaidAmount      int64
	CouponDiscount  int64
	Channel         vo.ChannelCode
	ChannelTradeNo  string
	SplitStatus     vo.SplitStatus
	Status          vo.OrderStatus
	ExpireAt        *time.Time
	PaidAt          *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// SplitRule 分账规则。
type SplitRule struct {
	ID            uint64
	RuleCode      string
	MerchantID    uint64
	RuleName      string
	Priority      int
	Conditions    string // JSON
	Allocations   string // JSON
	TriggerType   string // PAID / CONFIRM / SETTLE / MANUAL
	Status        int
	EffectiveFrom *time.Time
	EffectiveTo   *time.Time
}