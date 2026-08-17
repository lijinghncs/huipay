// Package splitcfg 集中管理分账模块的配置常量、退避策略与重试参数。
// 所有包统一引用此包，不散落魔数。
package splitcfg

import "time"

// ──────────────────────────────────────────────
// 重试与补偿参数
// ──────────────────────────────────────────────

// MaxChannelAttempts 通道调用与内部转账的最大尝试次数（文档 C1）。
const MaxChannelAttempts = 3

// MaxRetryAttempts 最大补偿重试次数（文档 B1：5 次）。
const MaxRetryAttempts = 5

// RetryBackoff 返回第 attempt 次重试的间隔（指数退避：30s→1m→2m→4m→8m，封顶16m）。
func RetryBackoff(attempt int) time.Duration {
	d := time.Duration(30<<(attempt-1)) * time.Second // 30s, 1m, 2m, 4m, 8m, 16m
	if d > 16*time.Minute {
		d = 16 * time.Minute
	}
	return d
}

// ──────────────────────────────────────────────
// 补偿调度参数
// ──────────────────────────────────────────────

// HangThreshold 悬挂判定阈值：PROCESSING 超过该时长未更新视为悬挂。
const HangThreshold = 10 * time.Minute

// BatchSize 单轮补偿处理上限。
const BatchSize = 50

// RecomputeInterval 重算调度默认间隔；与现有 09:00 对账错峰。
const RecomputeInterval = 10 * time.Minute

// LookbackWindow 增量窗口（重算时扫最近多久的变更）。
const LookbackWindow = 5 * time.Minute