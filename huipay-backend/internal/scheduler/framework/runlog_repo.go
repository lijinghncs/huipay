// 包 framework 提供定时任务监测基础设施：注册表 + 运行日志 + 统一执行契约。
package framework

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// RunStatus 运行状态。
const (
	RunRunning = "RUNNING"
	RunSuccess = "SUCCESS"
	RunFailed  = "FAILED"
	RunTimeout = "TIMEOUT"
)

// RunMode 运行方式。
const (
	ModeAuto   = "AUTO"
	ModeManual = "MANUAL"
)

// RunLogModel 运行日志表 GORM 模型（t_scheduler_run_log）。
type RunLogModel struct {
	ID           uint64     `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	Name         string     `gorm:"column:name;size:64;not null" json:"name"`
	InstanceID   string     `gorm:"column:instance_id;size:64;not null" json:"instance_id"`
	BizDate      *time.Time `gorm:"column:biz_date;type:date" json:"biz_date,omitempty"`
	RunMode      string     `gorm:"column:run_mode;size:16;not null" json:"run_mode"`
	Status       string     `gorm:"column:status;size:16;not null" json:"status"`
	StartedAt    time.Time  `gorm:"column:started_at;autoCreateTime" json:"started_at"`
	FinishedAt   *time.Time `gorm:"column:finished_at" json:"finished_at,omitempty"`
	DurationMs   int        `gorm:"column:duration_ms" json:"duration_ms"`
	RowsAffected int64      `gorm:"column:rows_affected" json:"rows_affected"`
	ErrorMessage string     `gorm:"column:error_message;type:text" json:"error_message,omitempty"`
	TraceID      string     `gorm:"column:trace_id;size:64" json:"trace_id,omitempty"`
}

// TableName 表名。
func (RunLogModel) TableName() string { return "t_scheduler_run_log" }

// RunLogRepo 运行日志仓储。
type RunLogRepo struct{ db *gorm.DB }

// NewRunLogRepo 构造 RunLogRepo。
func NewRunLogRepo(db *gorm.DB) *RunLogRepo { return &RunLogRepo{db: db} }

// DB 暴露底层连接（供事务复用）。
func (r *RunLogRepo) DB() *gorm.DB { return r.db }

// StartRun 写入 RUNNING 记录，返回 run_id。
func (r *RunLogRepo) StartRun(ctx context.Context, name, instanceID string, bizDate *time.Time, runMode string) (uint64, error) {
	m := &RunLogModel{
		Name:       name,
		InstanceID: instanceID,
		BizDate:    bizDate,
		RunMode:    runMode,
		Status:     RunRunning,
	}
	if err := r.db.WithContext(ctx).Create(m).Error; err != nil {
		return 0, err
	}
	return m.ID, nil
}

// FinishRun 成功结束：写 SUCCESS/finished_at/duration_ms/rows_affected。
func (r *RunLogRepo) FinishRun(ctx context.Context, id uint64, rowsAffected int64, duration time.Duration) error {
	now := time.Now()
	return r.db.WithContext(ctx).Model(&RunLogModel{}).Where("id = ?", id).Updates(map[string]any{
		"status":        RunSuccess,
		"finished_at":   now,
		"duration_ms":   int(duration.Milliseconds()),
		"rows_affected": rowsAffected,
	}).Error
}

// FailRun 失败结束：写 FAILED/finished_at/duration_ms/error_message。
func (r *RunLogRepo) FailRun(ctx context.Context, id uint64, errMsg string, duration time.Duration) error {
	now := time.Now()
	return r.db.WithContext(ctx).Model(&RunLogModel{}).Where("id = ?", id).Updates(map[string]any{
		"status":        RunFailed,
		"finished_at":   now,
		"duration_ms":   int(duration.Milliseconds()),
		"error_message": errMsg,
	}).Error
}

// RunFilter 运行日志筛选条件。
type RunFilter struct {
	Name       string
	Status     string
	Start      *time.Time
	End        *time.Time
	InstanceID string
}

// ListRuns 分页查询运行日志。
func (r *RunLogRepo) ListRuns(ctx context.Context, f RunFilter, offset, limit int) ([]RunLogModel, int64, error) {
	db := r.db.WithContext(ctx).Model(&RunLogModel{})
	if f.Name != "" {
		db = db.Where("name = ?", f.Name)
	}
	if f.Status != "" {
		db = db.Where("status = ?", f.Status)
	}
	if f.Start != nil {
		db = db.Where("started_at >= ?", f.Start)
	}
	if f.End != nil {
		db = db.Where("started_at <= ?", f.End)
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []RunLogModel
	if err := db.Order("started_at DESC").Offset(offset).Limit(limit).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// GetRun 运行日志详情。
func (r *RunLogRepo) GetRun(ctx context.Context, id uint64) (*RunLogModel, error) {
	var m RunLogModel
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&m).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &m, nil
}

// LatestByName 查询某任务最近一次运行记录。
func (r *RunLogRepo) LatestByName(ctx context.Context, name string) (*RunLogModel, error) {
	var m RunLogModel
	if err := r.db.WithContext(ctx).Where("name = ?", name).Order("started_at DESC").First(&m).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &m, nil
}

// RunLogged 轻量接入：执行 runner 并写运行日志（RUNNING→SUCCESS/FAILED），供保留自有 ticker 的旧调度器复用。
// db 为 nil 时仅执行 runner 不写日志；日志写入失败不阻断主流程；panic 兜底为 FAILED。
func RunLogged(ctx context.Context, db *gorm.DB, instanceID, name string, bizDate *time.Time, runner func() (int64, error)) (int64, error) {
	if db == nil {
		return safeRun(runner)
	}
	repo := NewRunLogRepo(db)
	started := time.Now()
	runID, err := repo.StartRun(ctx, name, instanceID, bizDate, ModeAuto)
	if err != nil {
		return safeRun(runner)
	}
	rows, runErr := safeRun(runner)
	duration := time.Since(started)
	if runErr != nil {
		_ = repo.FailRun(ctx, runID, runErr.Error(), duration)
	} else {
		_ = repo.FinishRun(ctx, runID, rows, duration)
	}
	return rows, runErr
}

// safeRun 执行 runner 并兜底捕获 panic（转为 error）。
func safeRun(runner func() (int64, error)) (rows int64, err error) {
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("scheduler run panic: %v", p)
		}
	}()
	return runner()
}