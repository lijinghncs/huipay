// 包 bootstrap 提供启动时的数据初始化。
// 通道在途资金户：外部虚拟源户，用于回调入账的 DEBIT 流出端，保证借贷平衡。
package bootstrap

import (
	"context"
	"errors"
	"fmt"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/huipay/huipay-backend/internal/account/service"
	"github.com/huipay/huipay-backend/internal/domain/vo"
)

// 通道在途资金户的 entity_code 常量（用于按通道查找）。
const (
	EntityCodeChannelSettlementWeChat = "channel_settlement_wechat"
	EntityCodeChannelSettlementAlipay = "channel_settlement_alipay"
)

// entity 表 GORM 模型（仅本包使用，避免引入实体仓储）。
type entity struct {
	ID         uint64 `gorm:"column:id;primaryKey;autoIncrement"`
	EntityCode string `gorm:"column:entity_code;size:32;uniqueIndex:uk_entity_code;not null"`
	EntityType string `gorm:"column:entity_type;size:32;not null"`
	Name       string `gorm:"column:name;size:128;not null"`
	Status     int    `gorm:"column:status;not null;default:1"`
}

// TableName 表名。
func (entity) TableName() string { return "t_entity" }

// SettleAccount 通道在途资金户信息。
type SettleAccount struct {
	EntityCode string
	WalletID   uint64
}

// SeedChannelSettlementWallets 启动时确保两个通道在途资金户 entity + wallet 存在。
// 幂等保证：
//  1. entity 通过 (entity_code) 唯一键兜底
//  2. wallet 通过 (entity_id, currency) 唯一键兜底
//
// 重复启动不会重复创建；返回微信结算户 wallet_id。
func SeedChannelSettlementWallets(ctx context.Context, db *gorm.DB, accountSvc *service.Service, logger *zap.Logger) (uint64, error) {
	accounts := []struct {
		code string
		name string
	}{
		{EntityCodeChannelSettlementWeChat, "微信通道在途资金户"},
		{EntityCodeChannelSettlementAlipay, "支付宝通道在途资金户"},
	}

	var wechatWalletID uint64
	for _, acc := range accounts {
		entityID, err := ensureEntity(ctx, db, acc.code, acc.name)
		if err != nil {
			return 0, err
		}
		w, err := accountSvc.EnsureWallet(ctx, entityID, vo.EntityPlatform)
		if err != nil {
			return 0, fmt.Errorf("seed wallet for %s: %w", acc.code, err)
		}
		logger.Info("seed settlement account", zap.String("entity_code", acc.code), zap.Uint64("wallet_id", w.ID))
		if acc.code == EntityCodeChannelSettlementWeChat {
			wechatWalletID = w.ID
		}
	}
	return wechatWalletID, nil
}

// ensureEntity 确保主体存在（按 entity_code 幂等），返回 entity_id。
func ensureEntity(ctx context.Context, db *gorm.DB, code, name string) (uint64, error) {
	var e entity
	err := db.WithContext(ctx).Where("entity_code = ?", code).First(&e).Error
	if err == nil {
		return e.ID, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, err
	}
	// 不存在则创建；唯一键冲突视为并发已创建
	e = entity{EntityCode: code, EntityType: string(vo.EntityPlatform), Name: name, Status: 1}
	if err := db.WithContext(ctx).Create(&e).Error; err != nil {
		if !errors.Is(err, gorm.ErrDuplicatedKey) {
			return 0, fmt.Errorf("create entity %s: %w", code, err)
		}
		if err := db.WithContext(ctx).Where("entity_code = ?", code).First(&e).Error; err != nil {
			return 0, err
		}
	}
	return e.ID, nil
}