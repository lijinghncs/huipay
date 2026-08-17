// Package ports 定义 recon 领域的出站接口（定义在消费侧，由装配层注入实现）。
// recon 核心（domain/compare/job）只依赖本包接口，不依赖任何业务域与基础设施实现。
package ports

import (
	"context"
	"time"

	"github.com/huipay/huipay-backend/internal/recon/domain"
)

// DiffStore 对账差异写入端口（recon/repository.DiffRepo 实现）。
// WriteOrderDiffs 幂等策略：按 diff_type + 商户 + 业务日期 清理未核销旧差异后批量写入；已核销差异保留。
type DiffStore interface {
	WritePrecheck(ctx context.Context, merchantID uint64, start, end time.Time, diffType string, detailJSON string) (uint64, error)
	WriteOrderDiffs(ctx context.Context, merchantID *uint64, bizDate time.Time, diffType string, rows []domain.Diff) (int, error)
}

// OrderSideFetcher 订单侧取数端口。
// SumForSplit / SumByStoreAndDate 为分账口径（排除分账执行单与账单订单）；
// ListPaidForChannel / MerchantsByOrderNos 为渠道对账口径。
type OrderSideFetcher interface {
	SumForSplit(ctx context.Context, merchantID uint64, start, end time.Time) (int64, error)
	SumByStoreAndDate(ctx context.Context, merchantID uint64, start, end time.Time) (map[string]int64, error)
	ListPaidForChannel(ctx context.Context, start, end time.Time) ([]domain.LocalOrder, error)
	MerchantsByOrderNos(ctx context.Context, orderNos []string) (map[string]uint64, error)
}

// StatsSideFetcher 日报侧取数 + 补跑端口。
type StatsSideFetcher interface {
	HasMissing(ctx context.Context, merchantID uint64, start, end time.Time) (bool, error)
	Backfill(ctx context.Context, start, end time.Time) (int64, error)
	Sum(ctx context.Context, merchantID uint64, start, end time.Time) (int64, error)
	Rows(ctx context.Context, merchantID uint64, start, end time.Time) (map[string]int64, error)
}

// JournalSideFetcher 账本分录取数端口（执行后对账本地侧）。
type JournalSideFetcher interface {
	SumByOrderForMerchants(ctx context.Context, merchantIDs []uint64, start, end time.Time) (map[string]int64, error)
}

// ExecutionSideFetcher 分账执行取数端口（执行后对账对端侧）。
type ExecutionSideFetcher interface {
	ListMerchantsWithExecution(ctx context.Context, start, end time.Time) ([]uint64, error)
	SumByOrder(ctx context.Context, merchantID uint64, start, end time.Time) ([]domain.OrderExecSum, error)
}

// ChannelBillFetcher 渠道账单获取端口（payment 域包装账单下载器实现）。
type ChannelBillFetcher interface {
	FetchBill(ctx context.Context, bizDate string) ([]domain.BillEntry, error)
}

// AuditRecorder 审计记录端口（写 t_split_audit，由装配层包装 split 审计仓储）。
type AuditRecorder interface {
	Record(ctx context.Context, bizType, bizID, action, operatorType string, operatorID uint64, detail any)
}

// RunLogger 运行日志补记端口（前置对账异常时补记对账中心运行日志）。
type RunLogger interface {
	Log(ctx context.Context, name string, bizDate time.Time, rows int64, runErr error)
}

// Observer 指标观测端口。
type Observer interface {
	ObservePrecheck(label string)
}
