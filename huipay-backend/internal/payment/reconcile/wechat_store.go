// 对账差异落库。
package reconcile

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// DiffModel 对账差异表 GORM 模型。
type DiffModel struct {
	ID            uint64     `gorm:"column:id;primaryKey;autoIncrement"`
	BizDate       string     `gorm:"column:biz_date"`        // DATE
	DiffType      string     `gorm:"column:diff_type;size:16"`
	OrderNo       *string    `gorm:"column:order_no;size:32"`
	TransactionID *string    `gorm:"column:transaction_id;size:64"`
	LocalAmount   *int64     `gorm:"column:local_amount"`
	ChannelAmount *int64     `gorm:"column:channel_amount"`
	Detail        string     `gorm:"column:detail"` // JSON
	ResolvedAt    *time.Time `gorm:"column:resolved_at"`
	CreatedAt     time.Time  `gorm:"column:created_at;autoCreateTime"`
}

// TableName 表名。
func (DiffModel) TableName() string { return "t_reconcile_diff" }

// SaveDiffs 将对账差异写入 t_reconcile_diff。
// 同一 biz_date 的老差异先清空再写入，保证对账可重复执行。
func SaveDiffs(ctx context.Context, db *gorm.DB, report *ReconcileReport) error {
	if report == nil {
		return nil
	}
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 幂等：清掉当日旧差异，避免重复累积
		if err := tx.Where("biz_date = ?", report.BizDate).Delete(&DiffModel{}).Error; err != nil {
			return fmt.Errorf("reconcile: clear old diff fail: %w", err)
		}
		rows := make([]DiffModel, 0, len(report.LongOrders)+len(report.ShortOrders)+len(report.MismatchOrders))
		rows = appendDiffRows(rows, report.BizDate, DiffLong, report.LongOrders)
		rows = appendDiffRows(rows, report.BizDate, DiffShort, report.ShortOrders)
		rows = appendDiffRows(rows, report.BizDate, DiffMismatch, report.MismatchOrders)
		if len(rows) > 0 {
			if err := tx.Create(&rows).Error; err != nil {
				return fmt.Errorf("reconcile: insert diff fail: %w", err)
			}
		}
		return nil
	})
}

// appendDiffRows 将 DiffEntry 列表转换为 DiffModel 行。
func appendDiffRows(rows []DiffModel, bizDate, diffType string, entries []DiffEntry) []DiffModel {
	for _, e := range entries {
		orderNo := e.OrderNo
		txnID := e.TransactionID
		localAmt := e.LocalAmount
		channelAmt := e.ChannelAmount
		rows = append(rows, DiffModel{
			BizDate:       bizDate,
			DiffType:      diffType,
			OrderNo:       strPtr(orderNo),
			TransactionID: strPtr(txnID),
			LocalAmount:   &localAmt,
			ChannelAmount: &channelAmt,
			Detail:        e.Detail,
		})
	}
	return rows
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}