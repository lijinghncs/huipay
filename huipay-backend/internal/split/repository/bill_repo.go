// 包 repository 提供分账单(t_split_bill)数据访问。
package repository

import (
	"context"
	"time"

	"gorm.io/gorm"
)

// SplitBillStatus 分账单状态。
const (
	BillPending  = "PENDING"  // 待审批
	BillApproved = "APPROVED" // 已通过（等待执行）
	BillRejected = "REJECTED" // 已驳回
	BillExecuted = "EXECUTED" // 已执行
)

// SplitBillItem 分账单明细（各门店可分金额）。
type SplitBillItem struct {
	ReceiverEntityID uint64 `json:"receiver_entity_id"`
	ReceiverType     string `json:"receiver_type"`
	ReceiverName     string `json:"receiver_name"`
	Amount           int64  `json:"amount"`
}

// SplitBillModel 分账单表 GORM 模型（t_split_bill）。
type SplitBillModel struct {
	ID          uint64         `gorm:"column:id;primaryKey;autoIncrement"`
	BatchNo     string         `gorm:"column:batch_no;size:64;uniqueIndex:uk_batch_no;not null"`
	MerchantID  uint64         `gorm:"column:merchant_id;not null"`
	RuleCode    string         `gorm:"column:rule_code;size:32;not null"`
	RuleName    string         `gorm:"column:rule_name;size:128;not null"`
	StartTime   time.Time      `gorm:"column:start_time;not null"`
	EndTime     time.Time      `gorm:"column:end_time;not null"`
	TotalAmount int64          `gorm:"column:total_amount;not null"`
	Detail      string         `gorm:"column:detail;type:json;not null"`
	Status      string         `gorm:"column:status;size:16;not null;default:PENDING"`
	ApprovedAt  *time.Time     `gorm:"column:approved_at"`
	ExecutedAt  *time.Time     `gorm:"column:executed_at"`
	CreatedAt   time.Time      `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt   time.Time      `gorm:"column:updated_at;autoUpdateTime"`
}

// TableName 表名。
func (SplitBillModel) TableName() string { return "t_split_bill" }

// SplitBillRepo 分账单仓储。
type SplitBillRepo struct{ db *gorm.DB }

// NewSplitBillRepo 构造 SplitBillRepo。
func NewSplitBillRepo(db *gorm.DB) *SplitBillRepo { return &SplitBillRepo{db: db} }

// Create 创建分账单。
func (r *SplitBillRepo) Create(ctx context.Context, m *SplitBillModel) error {
	return r.db.WithContext(ctx).Create(m).Error
}

// GetByBatchNo 按批次号 + 商户查询分账单。
func (r *SplitBillRepo) GetByBatchNo(ctx context.Context, batchNo string, merchantID uint64) (*SplitBillModel, error) {
	var row SplitBillModel
	if err := r.db.WithContext(ctx).
		Where("batch_no = ? AND merchant_id = ?", batchNo, merchantID).
		First(&row).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &row, nil
}

// ListByMerchant 分页查询商户分账单。
func (r *SplitBillRepo) ListByMerchant(ctx context.Context, merchantID uint64, offset, limit int) ([]SplitBillModel, int64, error) {
	db := r.db.WithContext(ctx).Model(&SplitBillModel{}).Where("merchant_id = ?", merchantID)
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []SplitBillModel
	if err := db.Order("created_at DESC").Offset(offset).Limit(limit).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// UpdateStatus 更新账单状态（含审批/执行时间戳）。
func (r *SplitBillRepo) UpdateStatus(ctx context.Context, id uint64, status string, extra map[string]any) error {
	fields := map[string]any{"status": status}
	for k, v := range extra {
		fields[k] = v
	}
	return r.db.WithContext(ctx).Model(&SplitBillModel{}).
		Where("id = ?", id).
		Updates(fields).Error
}

// GetStoreNames 批量查询门店名称（t_store.id -> name）。
func (r *SplitBillRepo) GetStoreNames(ctx context.Context, ids []uint64) (map[uint64]string, error) {
	out := make(map[uint64]string, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	var rows []struct {
		ID   uint64
		Name string
	}
	if err := r.db.WithContext(ctx).Table("t_store").
		Select("id, name").Where("id IN ?", ids).Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.ID] = row.Name
	}
	return out, nil
}

// GetEntityNames 批量查询主体名称（t_entity.id -> name）。
func (r *SplitBillRepo) GetEntityNames(ctx context.Context, ids []uint64) (map[uint64]string, error) {
	out := make(map[uint64]string, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	var rows []struct {
		ID   uint64
		Name string
	}
	if err := r.db.WithContext(ctx).Table("t_entity").
		Select("id, name").Where("id IN ?", ids).Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.ID] = row.Name
	}
	return out, nil
}