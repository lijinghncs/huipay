// 包 executor 实现分账执行（基于账户式账本）。
package executor

import (
	"context"
	"encoding/binary"
	"fmt"
	"hash/fnv"
	"time"

	"go.uber.org/zap"

	"github.com/huipay/huipay-backend/infra/notify"
	"github.com/huipay/huipay-backend/internal/account/ledger"
	"github.com/huipay/huipay-backend/internal/account/repository"
	"github.com/huipay/huipay-backend/internal/split/event"
	splitrepo "github.com/huipay/huipay-backend/internal/split/repository"
	"github.com/huipay/huipay-backend/internal/split/splitcfg"
	"github.com/huipay/huipay-backend/internal/domain/vo"
	"github.com/huipay/huipay-backend/internal/payment/router"
)

// maxAttempts 通道调用与内部转账的最大尝试次数。
const maxAttempts = splitcfg.MaxChannelAttempts

// 分账模式常量（C1 层：通道降级闸门，存 t_entity.split_mode）。
const (
	SplitModeAuto            = "AUTO"             // 有通道走通道；无通道降级本地入账并标记 degraded
	SplitModeLocalOnly       = "LOCAL_ONLY"       // 仅本地记账（不调通道，标记 degraded）
	SplitModeChannelRequired = "CHANNEL_REQUIRED" // 通道不可用即失败，不本地入账
)

// Allocation 分账分配单元。
type Allocation struct {
	Level      int
	EntityID   uint64
	EntityType vo.EntityType
	Amount     int64
}

// SplitExecutionModel 分账执行记录（t_split_execution）。
type SplitExecutionModel struct {
	ID               string     `gorm:"column:id;primaryKey"`
	OrderNo          string     `gorm:"column:order_no;size:32;not null"`
	ChannelReqNo     string     `gorm:"column:channel_req_no;size:64"`
	StoreID          *uint64    `gorm:"column:store_id"`
	RuleID           *uint64    `gorm:"column:rule_id"`
	ReceiverEntityID uint64     `gorm:"column:receiver_entity_id;not null"`
	ReceiverType     string     `gorm:"column:receiver_type;size:32;not null"`
	Amount           int64      `gorm:"column:amount;not null"`
	Level            int        `gorm:"column:level;not null;default:1"`
	Channel          string     `gorm:"column:channel;size:32"`
	ChannelSplitNo   string     `gorm:"column:channel_split_no;size:64"`
	Degraded         int        `gorm:"column:degraded;not null;default:0"`
	Status           string     `gorm:"column:status;size:16;not null"`
	RetryCount       int        `gorm:"column:retry_count;not null;default:0"`
	LastError        string     `gorm:"column:last_error;size:512"`
	ExecutedAt       *time.Time `gorm:"column:executed_at"`
}

// TableName 表名。
func (SplitExecutionModel) TableName() string { return "t_split_execution" }

// ExecuteRequest 分账执行请求。
type ExecuteRequest struct {
	MerchantID     uint64
	OrderNo        string
	SourceWallet   uint64 // 通常是平台备付金内部户
	Allocations    []Allocation
	StoreID        uint64 // 关联门店 ID（可选）
	RuleID         uint64 // 命中规则 ID（可选）
	Channel        vo.ChannelCode
	IdempotencyKey string
	TraceID        string
}

// Executor 分账执行器。
type Executor struct {
	walletRepo      *repository.WalletRepo
	journalRepo     *repository.JournalRepo
	orderStatusRepo *splitrepo.SplitOrderStatusRepo
	ledger          *ledger.Service
	channels        *router.Router
	alerter         notify.Alerter
	outboxRepo      *event.OutboxRepo
	logger          *zap.Logger
}

// NewExecutor 构造 Executor。
func NewExecutor(wr *repository.WalletRepo, jr *repository.JournalRepo, osr *splitrepo.SplitOrderStatusRepo, channels *router.Router, outbox *event.OutboxRepo, logger *zap.Logger) *Executor {
	return &Executor{
		walletRepo:      wr,
		journalRepo:     jr,
		orderStatusRepo: osr,
		ledger:          ledger.NewService(wr, jr, logger),
		channels:        channels,
		outboxRepo:      outbox,
		logger:          logger,
	}
}

// SetAlerter 注入告警器（可选；不注入则告警为空操作）。
func (e *Executor) SetAlerter(a notify.Alerter) {
	if a == nil {
		a = notify.NoopAlerter{}
	}
	e.alerter = a
}

// alert 安全触发告警（未注入时为空操作）。
func (e *Executor) alert(ctx context.Context, title, content string) {
	if e.alerter != nil {
		e.alerter.Alert(ctx, title, content)
	}
}

// executionID 确定性生成分账执行记录主键（同 (orderNo, entityID) 恒定，幂等防重复）。
func executionID(orderNo string, entityID uint64) string {
	h := fnv.New128a()
	h.Write([]byte(orderNo))
	binary.Write(h, binary.LittleEndian, entityID)
	return fmt.Sprintf("%016x", h.Sum(nil))
}

