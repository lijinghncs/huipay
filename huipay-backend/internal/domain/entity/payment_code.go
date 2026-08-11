// 包 entity 定义领域实体。
package entity

import (
	"time"

	"github.com/huipay/huipay-backend/internal/domain/vo"
)

// PaymentCode 收款码牌。
type PaymentCode struct {
	ID         uint64
	MerchantID uint64
	CodeID     string // 对外短码
	Status     vo.CodeStatus
	Remark     string
	CreatedAt  time.Time
	DisabledAt *time.Time
}