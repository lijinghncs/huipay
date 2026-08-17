// 包 framework 提供定时任务监测基础设施：注册表 + 运行日志 + 统一执行契约。
package framework

import (
	"context"
	"time"
)

// Runner 定时任务执行体：返回影响行数（无则 0）。bizDate 为 time.Time{} 表示无业务日期。
type Runner func(ctx context.Context, bizDate time.Time) (int64, error)

// RunOptions 单次执行的可选参数。
type RunOptions struct {
	// BizDate 计算业务日期；nil 表示无业务日期（传 time.Time{} 给 Runner）。
	BizDate func(now time.Time) time.Time
	// ShouldRun 命中判定；nil 表示每次 tick 都执行。
	ShouldRun func(now time.Time) bool
	// Timeout 单次执行超时（默认 10 分钟）；超时置 TIMEOUT。
	Timeout time.Duration
}

// WithTimeout 便捷构造超时选项。
func (o *RunOptions) effectiveTimeout() time.Duration {
	if o != nil && o.Timeout > 0 {
		return o.Timeout
	}
	return 10 * time.Minute
}