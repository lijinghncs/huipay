// Package repository 是 t_reconcile_diff 的唯一读写入口。
// 三层对账（前置/渠道/执行后）共用本仓储与 DiffModel，禁止绕过本包直写差异表。
package repository

import (
	"context"
	"time"

	"gorm.io/gorm"

	"github.com/huipay/huipay-backend/internal/recon/domain"
)

// DiffModel 对账差异表模型（t_reconcile_diff）。
type DiffModel struct {
	ID            uint64     `gorm:"primaryKey;autoIncrement;column:id" json:"id"`
	MerchantID    *uint64    `gorm:"not null;index:idx_recon_diff_merchant_biz,priority:1;column:merchant_id" json:"merchant_id"`
	StoreID       uint64     `gorm:"not null;default:0;column:store_id" json:"store_id"`
	BizDate       time.Time  `gorm:"type:date;not null;index:idx_recon_diff_merchant_biz,priority:2;column:biz_date" json:"biz_date"`
	DiffType      string     `gorm:"type:varchar(20);not null;index:idx_recon_diff_type;column:diff_type" json:"diff_type"`
	OrderNo       string     `gorm:"type:varchar(32);not null;default:'';column:order_no" json:"order_no"`
	TransactionID string     `gorm:"type:varchar(64);not null;default:'';column:transaction_id" json:"transaction_id"`
	LocalAmount   int64      `gorm:"not null;default:0;column:local_amount" json:"local_amount"`
	ChannelAmount int64      `gorm:"not null;default:0;column:channel_amount" json:"channel_amount"`
	Detail        string     `gorm:"type:text;column:detail" json:"detail"`
	ResolvedAt    *time.Time `gorm:"column:resolved_at" json:"resolved_at"`
	ResolvedBy    uint64     `gorm:"column:resolved_by" json:"resolved_by"`
	CreatedAt     time.Time  `gorm:"not null;default:CURRENT_TIMESTAMP;column:created_at" json:"created_at"`
}

func (DiffModel) TableName() string { return "t_reconcile_diff" }

// DiffRepo 对账差异仓储。
type DiffRepo struct{ db *gorm.DB }

func NewDiffRepo(db *gorm.DB) *DiffRepo { return &DiffRepo{db: db} }

// WritePrecheck 写入前置对账差异：单条汇总行，detailJSON 为比对明细（调用方负责序列化）。
// 幂等：先清同商户同区间同 diff_type 的未核销旧差异。返回写入行 ID。
func (r *DiffRepo) WritePrecheck(ctx context.Context, merchantID uint64, start, end time.Time, diffType string, detailJSON string) (uint64, error) {
	start = time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.Local)
	end = time.Date(end.Year(), end.Month(), end.Day(), 0, 0, 0, 0, time.Local)
	row := DiffModel{
		MerchantID: &merchantID,
		BizDate:    start,
		DiffType:   diffType,
		Detail:     detailJSON,
	}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("merchant_id = ? AND diff_type = ? AND resolved_at IS NULL AND biz_date >= ? AND biz_date <= ?",
			merchantID, diffType, start, end).Delete(&DiffModel{}).Error; err != nil {
			return err
		}
		return tx.Create(&row).Error
	})
	if err != nil {
		return 0, err
	}
	return row.ID, nil
}

// WriteOrderDiffs 写入订单级差异（渠道对账 / 执行后对账）。
// 幂等：按 diff_type + 商户 + 业务日期 清理未核销旧差异后批量写入；已核销差异保留，
// 且不影响同日其他 diff_type 的行（修复渠道对账曾按 biz_date 全量清理误删分账差异的缺陷）。
// merchantID 为 nil 表示无法归属商户（差异行 merchant_id 落 NULL）。
func (r *DiffRepo) WriteOrderDiffs(ctx context.Context, merchantID *uint64, bizDate time.Time, diffType string, rows []domain.Diff) (int, error) {
	if len(rows) == 0 {
		return 0, nil
	}
	bizDate = time.Date(bizDate.Year(), bizDate.Month(), bizDate.Day(), 0, 0, 0, 0, time.Local)
	models := make([]DiffModel, 0, len(rows))
	for _, d := range rows {
		models = append(models, DiffModel{
			MerchantID:    merchantID,
			StoreID:       d.StoreID,
			BizDate:       bizDate,
			DiffType:      diffType,
			OrderNo:       d.OrderNo,
			TransactionID: d.TransactionID,
			LocalAmount:   d.LocalAmount,
			ChannelAmount: d.RemoteAmount,
			Detail:        d.Detail,
		})
	}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		q := tx.Where("diff_type = ? AND biz_date = ? AND resolved_at IS NULL", diffType, bizDate)
		if merchantID != nil {
			q = q.Where("merchant_id = ?", *merchantID)
		} else {
			q = q.Where("merchant_id IS NULL")
		}
		if err := q.Delete(&DiffModel{}).Error; err != nil {
			return err
		}
		return tx.Create(&models).Error
	})
	if err != nil {
		return 0, err
	}
	return len(models), nil
}

// ListByMerchantAndType 管理端分页查询；merchantID=0 表示全部商户。
func (r *DiffRepo) ListByMerchantAndType(ctx context.Context, merchantID uint64, diffType string, resolved *bool, start, end time.Time, offset, limit int) ([]DiffModel, int64, error) {
	q := r.db.WithContext(ctx).Model(&DiffModel{})
	if merchantID > 0 {
		q = q.Where("merchant_id = ?", merchantID)
	}
	if diffType != "" {
		q = q.Where("diff_type = ?", diffType)
	}
	if resolved != nil {
		if *resolved {
			q = q.Where("resolved_at IS NOT NULL")
		} else {
			q = q.Where("resolved_at IS NULL")
		}
	}
	if !start.IsZero() {
		q = q.Where("biz_date >= ?", start)
	}
	if !end.IsZero() {
		q = q.Where("biz_date <= ?", end)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var items []DiffModel
	if err := q.Order("biz_date DESC, id DESC").Offset(offset).Limit(limit).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// ListForMerchant 商户端分页查询（merchant_id 必填）。
func (r *DiffRepo) ListForMerchant(ctx context.Context, merchantID uint64, diffType string, resolved *bool, start, end time.Time, offset, limit int) ([]DiffModel, int64, error) {
	q := r.db.WithContext(ctx).Model(&DiffModel{}).Where("merchant_id = ?", merchantID)
	if diffType != "" {
		q = q.Where("diff_type = ?", diffType)
	}
	if resolved != nil {
		if *resolved {
			q = q.Where("resolved_at IS NOT NULL")
		} else {
			q = q.Where("resolved_at IS NULL")
		}
	}
	if !start.IsZero() {
		q = q.Where("biz_date >= ?", start)
	}
	if !end.IsZero() {
		q = q.Where("biz_date <= ?", end)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var items []DiffModel
	if err := q.Order("biz_date DESC, id DESC").Offset(offset).Limit(limit).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// Resolve 核销差异（商户端，校验归属）。
func (r *DiffRepo) Resolve(ctx context.Context, merchantID, diffID uint64) (bool, error) {
	res := r.db.WithContext(ctx).Model(&DiffModel{}).
		Where("id = ? AND merchant_id = ? AND resolved_at IS NULL", diffID, merchantID).
		Updates(map[string]any{"resolved_at": time.Now(), "resolved_by": merchantID})
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
}

// ResolveById 核销差异（管理端，不校验商户归属）。
func (r *DiffRepo) ResolveById(ctx context.Context, diffID uint64) (bool, error) {
	res := r.db.WithContext(ctx).Model(&DiffModel{}).
		Where("id = ? AND resolved_at IS NULL", diffID).
		Updates(map[string]any{"resolved_at": time.Now()})
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
}
