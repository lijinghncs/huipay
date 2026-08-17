// 包 service 提供管理后台调度任务监测与手工触发服务。
package service

import (
	"context"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/huipay/huipay-backend/internal/scheduler/framework"
)

// SchedulerService 调度任务监测服务。
type SchedulerService struct {
	db     *gorm.DB
	logger *zap.Logger
}

// NewSchedulerService 构造 SchedulerService。
func NewSchedulerService(db *gorm.DB, logger *zap.Logger) *SchedulerService {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &SchedulerService{db: db, logger: logger}
}

// TaskVO 调度任务列表项（含最近一次运行信息）。
type TaskVO struct {
	Name            string     `json:"name"`
	DisplayName     string     `json:"display_name"`
	Description     string     `json:"description"`
	CronExpr        string     `json:"cron_expr"`
	IntervalSec     int        `json:"interval_sec"`
	Enabled         bool       `json:"enabled"`
	InstanceID      string     `json:"instance_id"`
	ManualSupported bool       `json:"manual_supported"`
	LastStatus      string     `json:"last_status,omitempty"`
	LastRunAt       *time.Time `json:"last_run_at,omitempty"`
	LastDurationMs  int        `json:"last_duration_ms,omitempty"`
	LastRows        int64      `json:"last_rows,omitempty"`
}

// ListTasks 返回已注册任务列表。
func (s *SchedulerService) ListTasks(ctx context.Context) ([]TaskVO, error) {
	regs, err := framework.ListRegistry(ctx, s.db)
	if err != nil {
		return nil, err
	}
	repo := framework.NewRunLogRepo(s.db)
	out := make([]TaskVO, 0, len(regs))
	for _, r := range regs {
		vo := TaskVO{
			Name:            r.Name,
			DisplayName:     r.DisplayName,
			Description:     r.Description,
			CronExpr:        r.CronExpr,
			IntervalSec:     r.IntervalSec,
			Enabled:         r.Enabled == 1,
			InstanceID:      r.InstanceID,
			ManualSupported: framework.HasManual(r.Name),
		}
		if last, lErr := repo.LatestByName(ctx, r.Name); lErr == nil && last != nil {
			vo.LastStatus = last.Status
			vo.LastRunAt = &last.StartedAt
			vo.LastDurationMs = last.DurationMs
			vo.LastRows = last.RowsAffected
		}
		out = append(out, vo)
	}
	return out, nil
}

// RunFilter 运行日志筛选。
type RunFilter struct {
	Name       string
	Status     string
	Start      *time.Time
	End        *time.Time
	Page       int
	PageSize   int
}

// ListRuns 分页查询运行日志。
type RunList struct {
	Items []framework.RunLogModel `json:"items"`
	Total int64                   `json:"total"`
}

func (s *SchedulerService) ListRuns(ctx context.Context, f RunFilter) (*RunList, error) {
	if f.Page < 1 {
		f.Page = 1
	}
	if f.PageSize <= 0 || f.PageSize > 200 {
		f.PageSize = 20
	}
	rows, total, err := framework.NewRunLogRepo(s.db).ListRuns(ctx, framework.RunFilter{
		Name:   f.Name,
		Status: f.Status,
		Start:  f.Start,
		End:    f.End,
	}, (f.Page-1)*f.PageSize, f.PageSize)
	if err != nil {
		return nil, err
	}
	return &RunList{Items: rows, Total: total}, nil
}

// GetRun 单次运行详情。
func (s *SchedulerService) GetRun(ctx context.Context, id uint64) (*framework.RunLogModel, error) {
	return framework.NewRunLogRepo(s.db).GetRun(ctx, id)
}

// TriggerTask 手工触发任务（异步执行）。
// bizDate 为可选业务日期（nil 表示使用任务默认日期）。
// 返回 supported=false 表示任务未注册手动执行体。
func (s *SchedulerService) TriggerTask(ctx context.Context, name string, bizDate *time.Time) (bool, error) {
	if !framework.HasManual(name) {
		return false, nil
	}
	// 异步执行，避免阻塞 HTTP 请求
	go func() {
		runCtx := context.Background()
		_, _, _ = framework.RunManual(runCtx, s.db, name, bizDate)
	}()
	return true, nil
}