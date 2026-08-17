// 包 repository 提供分账前置对账差异落库（t_reconcile_diff 扩展 SPLIT_PRECHECK 类型）。
package repository

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"gorm.io/gorm"
)

// 对账差异类型常量（应用层定义，与现有 LONG/SHORT/MISMATCH 共用表）。
const (
	DiffTypeLong          = "LONG"           // 长款（本地有 / 微信无）——支付对账
	DiffTypeShort         = "SHORT"          // 短款（微信有 / 本地无）——支付对账
	DiffTypeMismatch      = "MISMATCH"       // 金额不一致——支付对账
	DiffTypeSplitTotal    = "SPLIT_TOTAL"    // 分账前置：商户级总额不平
	DiffTypeSplitDetail   = "SPLIT_DETAIL"   // 分账前置：门店×日不平
	DiffTypeSplitPost     = "SPLIT_POST"     // 分账执行后：本地账本与执行记录不平
	DiffTypeSplitDegraded = "SPLIT_DEGRADED" // 分账执行后：降级订单（本地入账、通道未分）
)

// ReconcileDiffModel 对账差异表 GORM 模型（t_reconcile_diff）。
// 与 internal/payment/reconcile/wechat_store.go 中的 DiffModel 字段含义一致；
// 此处提供分账场景的写入与查询能力。
type ReconcileDiffModel struct {
	ID            uint64     `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	BizDate       time.Time  `gorm:"column:biz_date;type:date;not null" json:"biz_date"`
	MerchantID    *uint64    `gorm:"column:merchant_id" json:"merchant_id"`
	DiffType      string     `gorm:"column:diff_type;size:16;not null" json:"diff_type"`
	OrderNo       *string    `gorm:"column:order_no;size:32" json:"order_no"`
	TransactionID *string    `gorm:"column:transaction_id;size:64" json:"transaction_id"`
	LocalAmount   *int64     `gorm:"column:local_amount" json:"local_amount"`
	ChannelAmount *int64     `gorm:"column:channel_amount" json:"channel_amount"`
	Detail        string     `gorm:"column:detail;type:json" json:"detail"`
	ResolvedAt    *time.Time `gorm:"column:resolved_at" json:"resolved_at"`
	CreatedAt     time.Time  `gorm:"column:created_at;autoCreateTime" json:"created_at"`
}

// TableName 表名。
func (ReconcileDiffModel) TableName() string { return "t_reconcile_diff" }

// ReconcileDiffRepo 对账差异仓储。
type ReconcileDiffRepo struct{ db *gorm.DB }

// NewReconcileDiffRepo 构造仓储。
func NewReconcileDiffRepo(db *gorm.DB) *ReconcileDiffRepo {
	return &ReconcileDiffRepo{db: db}
}

// DB 暴露底层连接。
func (r *ReconcileDiffRepo) DB() *gorm.DB { return r.db }

// WriteSplitPrecheck 写入分账前置对账差异（多条）。返回写入条数。
// 若 merchantID/bizDate 范围内已存在 SPLIT_* 类型差异，先清空再写入（幂等）。
func (r *ReconcileDiffRepo) WriteSplitPrecheck(ctx context.Context, merchantID uint64, start, end time.Time, diffType string, diffs any) (uint64, error) {
	if diffType != DiffTypeSplitTotal && diffType != DiffTypeSplitDetail {
		return 0, nil
	}
	detailJSON, err := json.Marshal(diffs)
	if err != nil {
		return 0, err
	}

	var inserted uint64
	txErr := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 清空该商户 [start, end) 区间内同 diff_type 的旧差异（幂等）
		del := tx.Where("diff_type = ? AND merchant_id = ? AND biz_date >= ? AND biz_date < ?",
			diffType, merchantID, start, end).
			Delete(&ReconcileDiffModel{})
		if del.Error != nil {
			return del.Error
		}
		// 写一条汇总行（detail JSON 含具体差异明细）
		bizDate := start
		mid := merchantID
		row := &ReconcileDiffModel{
			BizDate:    bizDate,
			MerchantID: &mid,
			DiffType:   diffType,
			Detail:     string(detailJSON),
		}
		if err := tx.Create(row).Error; err != nil {
			return err
		}
		inserted = 1
		return nil
	})
	if txErr != nil {
		return 0, txErr
	}
	return inserted, nil
}

// ListByMerchantAndType 管理端分页查询（merchantID=0 表示全部）。
func (r *ReconcileDiffRepo) ListByMerchantAndType(ctx context.Context, merchantID uint64, diffType string, start, end time.Time, offset, limit int) ([]ReconcileDiffModel, int64, error) {
	db := r.db.WithContext(ctx).Model(&ReconcileDiffModel{}).
		Where("biz_date >= ? AND biz_date < ?", start, end)
	if merchantID > 0 {
		db = db.Where("merchant_id = ?", merchantID)
	}
	if diffType != "" {
		db = db.Where("diff_type = ?", diffType)
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []ReconcileDiffModel
	if err := db.Order("biz_date DESC, id DESC").Offset(offset).Limit(limit).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// WriteSplitPost 写入分账执行后对账差异（按订单逐条）。
// 幂等：先清空该商户该业务日同 diff_type 且未核销的旧差异，再批量写入。已核销差异保留。
func (r *ReconcileDiffRepo) WriteSplitPost(ctx context.Context, merchantID uint64, bizDate time.Time, diffType string, rows []ReconcileDiffModel) (int, error) {
	if diffType != DiffTypeSplitPost && diffType != DiffTypeSplitDegraded {
		return 0, nil
	}
	txErr := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("diff_type = ? AND merchant_id = ? AND biz_date = ? AND resolved_at IS NULL",
			diffType, merchantID, bizDate).
			Delete(&ReconcileDiffModel{}).Error; err != nil {
			return err
		}
		for i := range rows {
			rows[i].BizDate = bizDate
			rows[i].MerchantID = &merchantID
			rows[i].DiffType = diffType
			if err := tx.Create(&rows[i]).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if txErr != nil {
		return 0, txErr
	}
	return len(rows), nil
}

// GetByID 按 ID + 商户查询单一差异（nil 表示未找到或不属于该商户）。
func (r *ReconcileDiffRepo) GetByID(ctx context.Context, id, merchantID uint64) (*ReconcileDiffModel, error) {
	var m ReconcileDiffModel
	err := r.db.WithContext(ctx).Where("id = ? AND merchant_id = ?", id, merchantID).First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// ListForMerchant 商户端差异分页查询（强制商户隔离；resolved 非 nil 时按是否已核销过滤）。
func (r *ReconcileDiffRepo) ListForMerchant(ctx context.Context, merchantID uint64, diffType string, resolved *bool, start, end time.Time, offset, limit int) ([]ReconcileDiffModel, int64, error) {
	db := r.db.WithContext(ctx).Model(&ReconcileDiffModel{}).
		Where("merchant_id = ? AND biz_date >= ? AND biz_date < ?", merchantID, start, end)
	if diffType != "" {
		db = db.Where("diff_type = ?", diffType)
	}
	if resolved != nil {
		if *resolved {
			db = db.Where("resolved_at IS NOT NULL")
		} else {
			db = db.Where("resolved_at IS NULL")
		}
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []ReconcileDiffModel
	if err := db.Order("biz_date DESC, id DESC").Offset(offset).Limit(limit).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// Resolve 核销差异：置 resolved_at（乐观锁：仅未核销且归属该商户才生效）。
func (r *ReconcileDiffRepo) Resolve(ctx context.Context, id, merchantID uint64) (bool, error) {
	res := r.db.WithContext(ctx).Model(&ReconcileDiffModel{}).
		Where("id = ? AND merchant_id = ? AND resolved_at IS NULL", id, merchantID).
		Update("resolved_at", time.Now())
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
}

// ResolveById 管理端核销差异：跨商户，仅未核销才生效（乐观锁）。
func (r *ReconcileDiffRepo) ResolveById(ctx context.Context, id uint64) (bool, error) {
	res := r.db.WithContext(ctx).Model(&ReconcileDiffModel{}).
		Where("id = ? AND resolved_at IS NULL", id).
		Update("resolved_at", time.Now())
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
}