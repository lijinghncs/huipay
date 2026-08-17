// Package engine 定义对账 Job 的执行契约。
// 各 Job 自行编排 取数 → 比对 → 落库（经 ports 注入），调度器只依赖本契约驱动执行。
package engine

import (
	"context"
	"time"
)

// ScheduledJob 可由调度框架驱动的对账任务。
// Run 返回差异行数；bizDate 为 time.Time{} 表示无业务日期。
type ScheduledJob interface {
	Name() string
	Run(ctx context.Context, bizDate time.Time) (int64, error)
}
