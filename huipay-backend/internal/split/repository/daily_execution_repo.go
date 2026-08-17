// 包 repository 提供每日分账执行轨迹(t_split_daily_execution)数据访问。
package repository

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
)

// 每日执行状态常量。
const (
	DailyExecRunning = "RUNNING"
	DailyExecSuccess = "SUCCESS"
	DailyExecPartial = "PARTIAL"
	DailyExecFailed  = "FAILED"
)

// DailyExecutionModel 每日分账执行轨迹表 GORM 模型（t_split_daily_execution）。
type DailyExecutionModel struct {
	ID              uint64     `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	RunID           string     `gorm:"column:run_id;size:64;uniqueIndex:uk_run_id;not null" json:"run_id"`
	MerchantID      uint64     `gorm:"column:merchant_id;not null" json:"merchant_id"`
	BizDate         time.Time  `gorm:"column:biz_date;type:date;not null" json:"biz_date"`
	BatchNo         string     `gorm:"column:batch_no;size:64;not null" json:"batch_no"`
	Status          string     `gorm:"column:status;size:16;not null;default:RUNNING" json:"status"`
	StartedAt       time.Time  `gorm:"column:started_at;autoCreateTime" json:"started_at"`
	FinishedAt      *time.Time `gorm:"column:finished_at" json:"finished_at"`
	DurationMs      *int       `gorm:"column:duration_ms" json:"duration_ms"`
	ErrorCode       *string    `gorm:"column:error_code;size:64" json:"error_code"`
	ErrorMessage    *string    `gorm:"column:error_message;size:1024" json:"error_message"`
	ReconcileDiffID *uint64    `gorm:"column:reconcile_diff_id" json:"reconcile_diff_id"`
	OperatorType    string     `gorm:"column:operator_type;size:16;not null;default:SYSTEM" json:"operator_type"`
	OperatorID      uint64     `gorm:"column:operator_id;not null;default:0" json:"operator_id"`
}

// TableName 表名。
func (DailyExecutionModel) TableName() string { return "t_split_daily_execution" }

// DailyExecutionRepo 每日分账执行仓储。
type DailyExecutionRepo struct{ db *gorm.DB }

// NewDailyExecutionRepo 构造仓储。
func NewDailyExecutionRepo(db *gorm.DB) *DailyExecutionRepo {
	return &DailyExecutionRepo{db: db}
}

// DB 暴露底层连接。
func (r *DailyExecutionRepo) DB() *gorm.DB { return r.db }

// CreateWithRunID 创建 RUNNING 记录；uk_run_id 冲突时返回已有行（用于幂等）。
func (r *DailyExecutionRepo) CreateWithRunID(ctx context.Context, m *DailyExecutionModel) (*DailyExecutionModel, error) {
	err := r.db.WithContext(ctx).Create(m).Error
	if err == nil {
		return m, nil
	}
	// 唯一键冲突 → 返回已有记录
	existing, qErr := r.GetByRunID(ctx, m.RunID)
	if qErr != nil {
		return nil, err
	}
	if existing != nil {
		return existing, nil
	}
	return nil, err
}

// GetByRunID 按 run_id 查询。
func (r *DailyExecutionRepo) GetByRunID(ctx context.Context, runID string) (*DailyExecutionModel, error) {
	var row DailyExecutionModel
	if err := r.db.WithContext(ctx).Where("run_id = ?", runID).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &row, nil
}

// GetByMerchantBatch 按 merchant_id + batch_no 查询最新一条（同 batch 可被二次请求命中）。
func (r *DailyExecutionRepo) GetByMerchantBatch(ctx context.Context, merchantID uint64, batchNo string) (*DailyExecutionModel, error) {
	var row DailyExecutionModel
	if err := r.db.WithContext(ctx).
		Where("merchant_id = ? AND batch_no = ?", merchantID, batchNo).
		Order("started_at DESC").First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &row, nil
}

// MarkStatus 更新执行终态。
func (r *DailyExecutionRepo) MarkStatus(ctx context.Context, id uint64, status string, errorCode, errorMessage string, diffID *uint64, durationMs int) error {
	now := time.Now()
	updates := map[string]any{
		"status":      status,
		"finished_at": now,
		"duration_ms": durationMs,
	}
	if errorCode != "" {
		updates["error_code"] = errorCode
	}
	if errorMessage != "" {
		// 应用层已做截断与脱敏
		updates["error_message"] = errorMessage
	}
	if diffID != nil {
		updates["reconcile_diff_id"] = *diffID
	}
	return r.db.WithContext(ctx).Model(&DailyExecutionModel{}).
		Where("id = ?", id).Updates(updates).Error
}

// MarkStale watchdog：把超时仍 RUNNING 的执行标 STALE（仅日志告警，不改 status 常量语义）。
// 本函数仅记录到 audit（应用层处理），仓储不直接更新 status。
func (r *DailyExecutionRepo) ListStale(ctx context.Context, before time.Time, limit int) ([]DailyExecutionModel, error) {
	var rows []DailyExecutionModel
	if err := r.db.WithContext(ctx).
		Where("status = ? AND started_at < ?", DailyExecRunning, before).
		Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// ListByMerchantDateRange 管理端分页查询。
func (r *DailyExecutionRepo) ListByMerchantDateRange(ctx context.Context, merchantID uint64, start, end time.Time, status string, offset, limit int) ([]DailyExecutionModel, int64, error) {
	db := r.db.WithContext(ctx).Model(&DailyExecutionModel{}).
		Where("merchant_id = ? AND biz_date >= ? AND biz_date < ?", merchantID, start, end)
	if status != "" {
		db = db.Where("status = ?", status)
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []DailyExecutionModel
	if err := db.Order("started_at DESC").Offset(offset).Limit(limit).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// ListByBatchNo 管理端按批次号查询。
func (r *DailyExecutionRepo) ListByBatchNo(ctx context.Context, batchNo string, offset, limit int) ([]DailyExecutionModel, int64, error) {
	db := r.db.WithContext(ctx).Model(&DailyExecutionModel{}).Where("batch_no = ?", batchNo)
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []DailyExecutionModel
	if err := db.Order("started_at DESC").Offset(offset).Limit(limit).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// GetByID 管理端单次执行详情。
func (r *DailyExecutionRepo) GetByID(ctx context.Context, id uint64) (*DailyExecutionModel, error) {
	var row DailyExecutionModel
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &row, nil
}