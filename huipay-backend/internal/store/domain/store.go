// 包 domain 定义门店领域模型与常量。
package domain

import "time"

// StoreType 门店类型。
type StoreType string

// 门店类型枚举。
const (
	StoreTypeDirect    StoreType = "DIRECT"    // 直营
	StoreTypeFranchise StoreType = "FRANCHISE" // 加盟
	StoreTypePartner   StoreType = "PARTNER"   // 合作
)

// validStoreTypes 合法门店类型集合。
var ValidStoreTypes = map[StoreType]bool{
	StoreTypeDirect:    true,
	StoreTypeFranchise: true,
	StoreTypePartner:   true,
}

// Store 门店领域模型。
type Store struct {
	ID           uint64
	StoreCode    string
	MerchantID   uint64
	Name         string
	StoreType    string
	ContactPhone string
	Region       string
	Address      string
	Longitude    *float64
	Latitude     *float64
	Metadata     map[string]any
	Status       int
	CreatedAt    time.Time
	UpdatedAt    time.Time
}