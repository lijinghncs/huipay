// 包 framework 提供定时任务监测基础设施：注册表 + 运行日志 + 统一执行契约。
package framework

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// TaskConfig 调度任务注册信息。
type TaskConfig struct {
	Name        string // 唯一名
	DisplayName string // 中文名
	Description string
	CronExpr    string // 人类可读描述（每天02:00）
	IntervalSec int    // 轮询周期（秒）；0 表示非轮询（按时刻触发）
	Enabled     bool
}

// RegistryModel 调度注册表 GORM 模型（t_scheduler_registry）。
type RegistryModel struct {
	ID           uint64    `gorm:"column:id;primaryKey;autoIncrement"`
	Name         string    `gorm:"column:name;size:64;uniqueIndex:uk_name;not null"`
	DisplayName  string    `gorm:"column:display_name;size:128;not null"`
	Description  string    `gorm:"column:description;size:512"`
	CronExpr     string    `gorm:"column:cron_expr;size:64"`
	IntervalSec  int       `gorm:"column:interval_sec"`
	Enabled      int       `gorm:"column:enabled;not null;default:1"`
	InstanceID   string    `gorm:"column:instance_id;size:64;not null"`
	RegisteredAt time.Time `gorm:"column:registered_at;autoCreateTime"`
}

// TableName 表名。
func (RegistryModel) TableName() string { return "t_scheduler_registry" }

// Handle 一个已注册调度任务的执行句柄。
type Handle struct {
	cfg        TaskConfig
	db         *gorm.DB
	logger     *zap.Logger
	instanceID string
	interval   time.Duration
	mu         sync.Mutex
}

// InstanceID 返回本次进程实例 ID。
func (h *Handle) InstanceID() string { return h.instanceID }

// Config 返回注册信息。
func (h *Handle) Config() TaskConfig { return h.cfg }

// globalInstanceID 生成实例 ID（HOSTNAME+PID）。
func globalInstanceID() string {
	host, _ := os.Hostname()
	return fmt.Sprintf("%s-%d", host, os.Getpid())
}

// GlobalInstanceID 返回全局实例 ID（供保留自有 ticker 的旧调度器写运行日志时用）。
func GlobalInstanceID() string { return globalInstanceID() }

// ListRegistry 查询已注册调度任务列表（t_scheduler_registry）。
func ListRegistry(ctx context.Context, db *gorm.DB) ([]RegistryModel, error) {
	var rows []RegistryModel
	if err := db.WithContext(ctx).Model(&RegistryModel{}).
		Order("id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// Register 注册调度任务并写入 t_scheduler_registry（ON DUPLICATE KEY UPDATE 更新描述/周期/实例）。
// 返回 *Handle，供 Start 启动。db 为 nil 时仅记录进程内注册表，不落库。
func Register(db *gorm.DB, logger *zap.Logger, cfg TaskConfig) *Handle {
	if logger == nil {
		logger = zap.NewNop()
	}
	h := &Handle{
		cfg:        cfg,
		db:         db,
		logger:     logger,
		instanceID: globalInstanceID(),
	}
	if cfg.IntervalSec <= 0 {
		h.interval = 1 * time.Minute // 非轮询型默认 1min tick 用于命中时刻窗口
	} else {
		h.interval = time.Duration(cfg.IntervalSec) * time.Second
	}
	if db != nil {
		enabled := 1
		if !cfg.Enabled {
			enabled = 0
		}
		m := &RegistryModel{
			Name:        cfg.Name,
			DisplayName: cfg.DisplayName,
			Description: cfg.Description,
			CronExpr:    cfg.CronExpr,
			IntervalSec: cfg.IntervalSec,
			Enabled:     enabled,
			InstanceID:  h.instanceID,
		}
		if err := db.WithContext(context.Background()).Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "name"}},
			DoUpdates: clause.AssignmentColumns([]string{"display_name", "description", "cron_expr", "interval_sec", "enabled", "instance_id"}),
		}).Create(m).Error; err != nil {
			logger.Warn("register scheduler to db fail", zap.String("name", cfg.Name), zap.Error(err))
		}
	}
	return h
}

// Start 启动调度循环，阻塞直到 ctx 取消。runlog 写入失败不阻断主流程。
func (h *Handle) Start(ctx context.Context, runner Runner, opts ...RunOptions) {
	var o *RunOptions
	if len(opts) > 0 {
		o = &opts[0]
	}
	timeout := o.effectiveTimeout()
	ticker := time.NewTicker(h.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			if o != nil && o.ShouldRun != nil && !o.ShouldRun(now) {
				continue
			}
			h.executeOnce(ctx, runner, o, now, timeout)
		}
	}
}

// executeOnce 执行一次并写运行日志（RUNNING→SUCCESS/FAILED/TIMEOUT），panic 兜底。
func (h *Handle) executeOnce(ctx context.Context, runner Runner, o *RunOptions, now time.Time, timeout time.Duration) {
	started := time.Now()
	var bizDate *time.Time
	if o != nil && o.BizDate != nil {
		bd := o.BizDate(now)
		if !bd.IsZero() {
			bizDate = &bd
		}
	}

	// 写 RUNNING（失败仅 warn，不阻断）
	runID, err := h.startRun(ctx, bizDate)
	if err != nil {
		h.logger.Warn("scheduler start run log fail", zap.String("name", h.cfg.Name), zap.Error(err))
	}

	// 带超时执行 + panic 兜底
	rows, runErr := h.runWithRecover(ctx, runner, bizDate, timeout)

	duration := time.Since(started)
	if o != nil && o.BizDate != nil {
		// 空 bizDate 时 Runner 收到 time.Time{}
		bd := o.BizDate(now)
		if !bd.IsZero() {
			bizDate = &bd
		}
	}
	_ = bizDate

	if runErr != nil {
		h.logger.Warn("scheduler run fail",
			zap.String("name", h.cfg.Name), zap.Duration("duration", duration), zap.Error(runErr))
		if runID > 0 {
			if err := h.failRun(ctx, runID, runErr.Error(), duration); err != nil {
				h.logger.Warn("scheduler fail run log fail", zap.String("name", h.cfg.Name), zap.Error(err))
			}
		}
		return
	}
	if runID > 0 {
		if err := h.finishRun(ctx, runID, rows, duration); err != nil {
			h.logger.Warn("scheduler finish run log fail", zap.String("name", h.cfg.Name), zap.Error(err))
		}
	}
}

// startRun/finishRun/failRun 仅在 Handle 持有 runlog 仓储连接时可用；无 db 时降级为空操作。
const noRunID = uint64(0)

func (h *Handle) startRun(ctx context.Context, bizDate *time.Time) (uint64, error) {
	if h.db == nil {
		return noRunID, nil
	}
	repo := NewRunLogRepo(h.db)
	return repo.StartRun(ctx, h.cfg.Name, h.instanceID, bizDate, ModeAuto)
}

func (h *Handle) finishRun(ctx context.Context, id uint64, rows int64, duration time.Duration) error {
	if h.db == nil {
		return nil
	}
	return NewRunLogRepo(h.db).FinishRun(ctx, id, rows, duration)
}

func (h *Handle) failRun(ctx context.Context, id uint64, errMsg string, duration time.Duration) error {
	if h.db == nil {
		return nil
	}
	return NewRunLogRepo(h.db).FailRun(ctx, id, errMsg, duration)
}

// runWithRecover 带超时与 panic 兜底执行 Runner。
func (h *Handle) runWithRecover(ctx context.Context, runner Runner, bizDate *time.Time, timeout time.Duration) (rows int64, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
			h.logger.Error("scheduler run panic", zap.String("name", h.cfg.Name), zap.Any("panic", r))
		}
	}()
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	done := make(chan struct{})
	var bd time.Time
	if bizDate != nil {
		bd = *bizDate
	}
	go func() {
		defer close(done)
		rows, err = runner(runCtx, bd)
	}()
	select {
	case <-done:
		return rows, err
	case <-runCtx.Done():
		return 0, fmt.Errorf("scheduler run timeout after %s", timeout)
	}
}