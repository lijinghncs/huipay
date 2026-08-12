// 包 model 定义 GORM 模型。
package model

import (
	"time"

	"github.com/huipay/huipay-backend/internal/domain/vo"
)

// OrderModel 订单表 GORM 模型。
type OrderModel struct {
	ID              uint64         `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	OrderNo         string         `gorm:"column:order_no;size:32;uniqueIndex:uk_order_no;not null" json:"order_no"`
	MerchantOrderNo string         `gorm:"column:merchant_order_no;size:64;not null" json:"merchant_order_no"`
	MerchantID      uint64         `gorm:"column:merchant_id;not null" json:"merchant_id"`
	StoreID         *uint64        `gorm:"column:store_id" json:"store_id"` // 关联门店 ID（可选）
	CodeID          string         `gorm:"column:code_id;size:16" json:"code_id"` // 来源收款码牌短码
	// 非持久字段：关联门店名称（列表查询 JOIN 填充）
	StoreName       string         `gorm:"-" json:"store_name,omitempty"`
	ParentOrderNo   string         `gorm:"column:parent_order_no;size:32" json:"parent_order_no"`
	OrderType       string         `gorm:"column:order_type;size:32;not null;default:PAYMENT" json:"order_type"`
	Amount          int64          `gorm:"column:amount;not null" json:"amount"`
	PaidAmount      int64          `gorm:"column:paid_amount;not null;default:0" json:"paid_amount"`
	CouponDiscount  int64          `gorm:"column:coupon_discount;not null;default:0" json:"coupon_discount"`
	Channel         vo.ChannelCode `gorm:"column:channel;size:32" json:"channel"`
	ChannelTradeNo  string         `gorm:"column:channel_trade_no;size:64" json:"channel_trade_no"`
	SplitStatus     string         `gorm:"column:split_status;size:16;not null;default:PENDING" json:"split_status"`
	Status          string         `gorm:"column:status;size:16;not null;default:CREATED" json:"status"`
	ExpireAt        *time.Time     `gorm:"column:expire_at" json:"expire_at"`
	PaidAt          *time.Time     `gorm:"column:paid_at" json:"paid_at"`
	ClosedAt        *time.Time     `gorm:"column:closed_at" json:"closed_at"`
	CreatedAt       time.Time      `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt       time.Time      `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
	DeletedAt       *time.Time     `gorm:"column:deleted_at;index" json:"deleted_at"`
}

// TableName 表名。
func (OrderModel) TableName() string { return "t_order" }
