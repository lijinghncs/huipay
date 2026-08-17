// 包 repository 提供账单-业务日期关联(t_split_bill_biz_date)数据访问。
// 用于 Prechecker 反向匹配：判断某商户某日是否被已 EXECUTED 账单覆盖。
package repository

import (
	"context"
	"time"

	"gorm.io/gorm"
)

// BillBizDateModel GORM 模型。
type BillBizDateModel struct {
	BillID  uint64    `gorm:"column:bill_id;primaryKey" json:"bill_id"`
	BizDate time.Time `gorm:"column:biz_date;type:date;primaryKey" json:"biz_date"`
}

// TableName 表名。
func (BillBizDateModel) TableName() string { return "t_split_bill_biz_date" }

// BillBizDateRepo 关联表仓储。
type BillBizDateRepo struct{ db *gorm.DB }

// NewBillBizDateRepo 构造仓储。
func NewBillBizDateRepo(db *gorm.DB) *BillBizDateRepo {
	return &BillBizDateRepo{db: db}
}

// DB 暴露底层连接。
func (r *BillBizDateRepo) DB() *gorm.DB { return r.db }

// Bind 批量绑定 bill 与 biz_dates。重复键忽略（idempotent）。
func (r *BillBizDateRepo) Bind(ctx context.Context, billID uint64, bizDates []time.Time) error {
	if len(bizDates) == 0 {
		return nil
	}
	rows := make([]BillBizDateModel, 0, len(bizDates))
	for _, d := range bizDates {
		rows = append(rows, BillBizDateModel{BillID: billID, BizDate: d})
	}
	// INSERT IGNORE 保证幂等（重复 (bill_id, biz_date) 跳过）
	return r.db.WithContext(ctx).
		Clauses(clauseIgnore()).
		Create(&rows).Error
}

// ListBillsByDate 查询某商户某日被哪些账单覆盖。
func (r *BillBizDateRepo) ListBillsByDate(ctx context.Context, merchantID uint64, bizDate time.Time) ([]uint64, error) {
	var ids []uint64
	err := r.db.WithContext(ctx).
		Table("t_split_bill_biz_date bd").
		Select("bd.bill_id").
		Joins("INNER JOIN t_split_bill sb ON sb.id = bd.bill_id").
		Where("sb.merchant_id = ? AND bd.biz_date = ?", merchantID, bizDate).
		Pluck("bd.bill_id", &ids).Error
	return ids, err
}

// ListBillNosByDateRange 查询 [from, to) 区间内某商户被账单覆盖的 (batch_no, biz_date) 列表。
func (r *BillBizDateRepo) ListBillNosByDateRange(ctx context.Context, merchantID uint64, from, to time.Time) ([]BillBizDateEntry, error) {
	type row struct {
		BatchNo string    `gorm:"column:batch_no"`
		BizDate time.Time `gorm:"column:biz_date"`
	}
	var rows []row
	err := r.db.WithContext(ctx).
		Table("t_split_bill_biz_date bd").
		Select("sb.batch_no, bd.biz_date").
		Joins("INNER JOIN t_split_bill sb ON sb.id = bd.bill_id").
		Where("sb.merchant_id = ? AND bd.biz_date >= ? AND bd.biz_date < ?", merchantID, from, to).
		Order("bd.biz_date ASC").
		Scan(&rows).Error
	out := make([]BillBizDateEntry, 0, len(rows))
	for _, r := range rows {
		out = append(out, BillBizDateEntry{BatchNo: r.BatchNo, BizDate: r.BizDate})
	}
	return out, err
}

// BillBizDateEntry 查询辅助结构。
type BillBizDateEntry struct {
	BatchNo string
	BizDate time.Time
}