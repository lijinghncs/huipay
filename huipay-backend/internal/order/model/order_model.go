// 包 model 定义 GORM 模型。
package model

import (
	"time"

	"github.com/huipay/huipay-backend/internal/domain/vo"
)

// OrderModel 订单表 GORM 模型。
type OrderModel struct {
	ID              uint64         `gorm:"column:id;primaryKey;autoIncrement"`
	OrderNo         string         `gorm:"column:order_no;size:32;uniqueIndex:uk_order_no;not null"`
	MerchantOrderNo string         `gorm:"column:merchant_order_no;size:64;not null"`
	MerchantID      uint64         `gorm:"column:merchant_id;not null"`
	CodeID          string         `gorm:"column:code_id;size:16"` // 来源收款码牌短码
	ParentOrderNo   string         `gorm:"column:parent_order_no;size:32"`
	OrderType       string         `gorm:"column:order_type;size:32;not null;default:PAYMENT"`
	Amount          int64          `gorm:"column:amount;not null"`
	PaidAmount      int64          `gorm:"column:paid_amount;not null;default:0"`
	CouponDiscount  int64          `gorm:"column:coupon_discount;not null;default:0"`
	Channel         vo.ChannelCode `gorm:"column:channel;size:32"`
	ChannelTradeNo  string         `gorm:"column:channel_trade_no;size:64"`
	SplitStatus     string         `gorm:"column:split_status;size:16;not null;default:PENDING"`
	Status          string         `gorm:"column:status;size:16;not null;default:CREATED"`
	ExpireAt        *time.Time     `gorm:"column:expire_at"`
	PaidAt          *time.Time     `gorm:"column:paid_at"`
	ClosedAt        *time.Time     `gorm:"column:closed_at"`
	CreatedAt       time.Time      `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt       time.Time      `gorm:"column:updated_at;autoUpdateTime"`
	DeletedAt       *time.Time     `gorm:"column:deleted_at;index"`
}

// TableName 表名。
func (OrderModel) TableName() string { return "t_order" }