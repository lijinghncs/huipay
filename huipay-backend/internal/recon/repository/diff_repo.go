// Package repository 是 t_reconcile_diff 的唯一读写入口。
// 三层对账（前置 / 渠道 / 执行后）共用本仓储；幂等策略统一为
// 「按 diff_type（+商户+业务日期）清理未核销后重写」，已核销历史保留。
package repository

import (
	"context"
	"time"

	"gorm.io/gorm"

	"github.com/huipay/huipay-backend/internal/recon/domain"
)

// DiffModel 对账差异表模型（映射 t_reconcile_diff 现有表结构，不新增列）。
type DiffModel struct {
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

func (DiffModel) TableName() string { return "t_reconcile_diff" }

// DiffStore t_reconcile_diff 仓储。
type DiffStore struct{ db *gorm.DB }

func NewDiffStore(db *gorm.DB) *DiffStore { return &DiffStore{db: db} }

// WritePrecheck 写入前置对账差异：单条汇总行，detail 为比对明细 JSON。
// 幂等：先清同商户同区间同 diff_type 的未核销差异（保留已核销历史）。返回写入行 ID。
func (r *DiffStore) WritePrecheck(ctx context.Context, merchantID uint64, start, end time.Time, diffType string, detailJSON string) (uint64, error) {
	mid := merchantID
	model := DiffModel{
		BizDate:    start,
		MerchantID: &mid,
		DiffType:   diffType,
		Detail:     detailJSON,
	}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("merchant_id = ? AND diff_type = ? AND biz_date >= ? AND biz_date < ? AND resolved_at IS NULL",
			merchantID, diffType, start, end).Delete(&DiffModel{}).Error; err != nil {
			return err
		}
		return tx.Create(&model).Error
	})
	if err != nil {
		return 0, err
	}
	return model.ID, nil
}

// WriteOrderDiffs 写入订单级差异（渠道对账 / 执行后对账）。
// 幂等：先清同商户（merchantID 为 nil 时清 merchant_id IS NULL）同业务日期同 diff_type 的未核销差异，再批量写入。
func (r *DiffStore) WriteOrderDiffs(ctx context.Context, merchantID *uint64, bizDate time.Time, diffType string, rows []domain.Diff) (int, error) {
	if len(rows) == 0 {
		return 0, nil
	}
	models := make([]DiffModel, 0, len(rows))
	for _, d := range rows {
		models = append(models, toModel(merchantID, bizDate, diffType, d))
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

// GetByID 按 ID 查询差异行（merchantID>0 时限定归属）。
func (r *DiffStore) GetByID(ctx context.Context, id uint64, merchantID uint64) (*DiffModel, error) {
	var m DiffModel
	q := r.db.WithContext(ctx).Where("id = ?", id)
	if merchantID > 0 {
		q = q.Where("merchant_id = ?", merchantID)
	}
	if err := q.First(&m).Error; err != nil {
		return nil, err
	}
	return &m, nil
}

// ListByMerchantAndType 管理端分页查询（merchantID=0 表示全部）。
func (r *DiffStore) ListByMerchantAndType(ctx context.Context, merchantID uint64, diffType string, resolved *bool, start, end time.Time, offset, limit int) ([]DiffModel, int64, error) {
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
	err := q.Order("biz_date DESC").Order("id DESC").Offset(offset).Limit(limit).Find(&items).Error
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// ListForMerchant 商户端分页查询（仅本商户）。
func (r *DiffStore) ListForMerchant(ctx context.Context, merchantID uint64, diffType string, resolved *bool, start, end time.Time, offset, limit int) ([]DiffModel, int64, error) {
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
	err := q.Order("biz_date DESC").Order("id DESC").Offset(offset).Limit(limit).Find(&items).Error
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// Resolve 核销差异（仅未核销可核销）。
func (r *DiffStore) Resolve(ctx context.Context, merchantID, diffID uint64) (bool, error) {
	now := time.Now()
	res := r.db.WithContext(ctx).Model(&DiffModel{}).
		Where("id = ? AND merchant_id = ? AND resolved_at IS NULL", diffID, merchantID).
		Update("resolved_at", now)
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
}

// ResolveByID 管理端核销（不限商户）。
func (r *DiffStore) ResolveByID(ctx context.Context, id uint64) (bool, error) {
	now := time.Now()
	res := r.db.WithContext(ctx).Model(&DiffModel{}).
		Where("id = ? AND resolved_at IS NULL", id).
		Update("resolved_at", now)
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
}

// toModel 领域差异行 → 表模型（金额指针直传，保留 NULL/0 语义）。
func toModel(merchantID *uint64, bizDate time.Time, diffType string, d domain.Diff) DiffModel {
	m := DiffModel{
		BizDate:       bizDate,
		MerchantID:    merchantID,
		DiffType:      diffType,
		Detail:        d.Detail,
		LocalAmount:   d.LocalAmount,
		ChannelAmount: d.RemoteAmount,
	}
	if d.OrderNo != "" {
		v := d.OrderNo
		m.OrderNo = &v
	}
	if d.TransactionID != "" {
		v := d.TransactionID
		m.TransactionID = &v
	}
	return m
}
