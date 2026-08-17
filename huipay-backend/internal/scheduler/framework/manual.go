// 包 framework 提供定时任务监测基础设施：注册表 + 运行日志 + 统一执行契约。
package framework

import (
	"context"
	"fmt"
	"sync"
	"time"

	"gorm.io/gorm"
)

// manualRunners 按任务名注册的「手动触发」执行体，供 admin 端强制重跑。
var manualRunners = struct {
	sync.Mutex
	m map[string]func(context.Context, time.Time) (int64, error)
}{m: make(map[string]func(context.Context, time.Time) (int64, error))}

// RegisterManual 注册任务的手动触发执行体（由各调度器在初始化时调用）。
func RegisterManual(name string, fn func(ctx context.Context, bizDate time.Time) (int64, error)) {
	manualRunners.Lock()
	defer manualRunners.Unlock()
	manualRunners.m[name] = fn
}

// HasManual 判断任务是否支持手动触发。
func HasManual(name string) bool {
	manualRunners.Lock()
	defer manualRunners.Unlock()
	_, ok := manualRunners.m[name]
	return ok
}

// RunManual 手动触发执行任务并写运行日志（run_mode=MANUAL）。
// 同步执行，返回 run_id 与影响行数。db 为 nil 时仅执行不写日志。
func RunManual(ctx context.Context, db *gorm.DB, name string, bizDate *time.Time) (uint64, int64, error) {
	manualRunners.Lock()
	fn, ok := manualRunners.m[name]
	manualRunners.Unlock()
	if !ok {
		return 0, 0, nil // 未注册，调用方按"任务不支持手动触发"处理
	}
	if db == nil {
		rows, err := wrapRecover(func() (int64, error) { return fn(ctx, zeroOr(bizDate)) })
		return 0, rows, err
	}
	repo := NewRunLogRepo(db)
	runID, err := repo.StartRun(ctx, name, GlobalInstanceID(), bizDate, ModeManual)
	if err != nil {
		return 0, 0, err
	}
	started := time.Now()
	rows, runErr := wrapRecover(func() (int64, error) { return fn(ctx, zeroOr(bizDate)) })
	duration := time.Since(started)
	if runErr != nil {
		_ = repo.FailRun(ctx, runID, runErr.Error(), duration)
	} else {
		_ = repo.FinishRun(ctx, runID, rows, duration)
	}
	return runID, rows, runErr
}

func zeroOr(bizDate *time.Time) time.Time {
	if bizDate == nil {
		return time.Time{}
	}
	return *bizDate
}

func wrapRecover(runner func() (int64, error)) (rows int64, err error) {
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("manual run panic: %v", p)
		}
	}()
	return runner()
}