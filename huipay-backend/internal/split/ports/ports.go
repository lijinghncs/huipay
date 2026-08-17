// Package ports 定义分账域的出站接口（依赖倒置用）。
//
// 接口定义在消费侧（分账域），实现侧在基础设施层。
// Go 隐式实现，无需在实现侧声明 implements。
package ports

import (
	"context"
	"time"

	"github.com/huipay/huipay-backend/internal/domain/vo"
	"github.com/huipay/huipay-backend/internal/split/executor"
	"github.com/huipay/huipay-backend/internal/split/recon"
)

// WalletResolver 商户钱包查询（跨域接口，替代 account.Service）。
// 返回钱包 ID；钱包不存在时返回 (0, error)。
type WalletResolver interface {
	GetWalletByEntityType(ctx context.Context, entityID uint64, entityType vo.EntityType) (uint64, error)
}

// Executor 分账执行器（只读查询 + 执行）。
type Executor interface {
	Execute(ctx context.Context, req *executor.ExecuteRequest) error
	ListByOrderNo(ctx context.Context, orderNo string) ([]executor.SplitExecutionModel, error)
	ListByMerchant(ctx context.Context, merchantID uint64, offset, limit int, f executor.SplitExecutionFilter) ([]executor.SplitExecutionSummary, int64, error)
	ListByOrderNoForMerchant(ctx context.Context, merchantID uint64, orderNo string) ([]executor.SplitExecutionModel, error)
	ListByOrderNoWithReceiver(ctx context.Context, merchantID uint64, orderNo string) ([]executor.SplitExecutionDetail, error)
}

// Prechecker 前置对账器。
type Prechecker interface {
	Check(ctx context.Context, merchantID uint64, start, end time.Time) (*recon.CheckResult, error)
}