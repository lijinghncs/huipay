package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/huipay/huipay-backend/infra/notify"
	"github.com/huipay/huipay-backend/infra/prom"
	"github.com/huipay/huipay-backend/internal/account/ledger"
	"github.com/huipay/huipay-backend/internal/account/repository"
	"github.com/huipay/huipay-backend/internal/domain/vo"
	"github.com/huipay/huipay-backend/internal/payment/router"
	"github.com/huipay/huipay-backend/internal/payment/channel"
	splitrepo "github.com/huipay/huipay-backend/internal/split/repository"
	"github.com/huipay/huipay-backend/internal/split/splitcfg"
	"github.com/huipay/huipay-backend/internal/split/state"
)

const maxAttempts = splitcfg.MaxChannelAttempts

const (
	SplitModeLocalOnly   = "LOCAL_ONLY"
	SplitModeChannelReq  = "CHANNEL_REQUIRED"
	SplitModeAuto        = "AUTO"
)

type Allocation struct {
	EntityID      uint64  `json:"entity_id"`
	EntityType    string  `json:"entity_type"`
	EntityName    string  `json:"entity_name"`
	Amount        int64   `json:"amount"`
	Level         int     `json:"level"`
	ReceiverScope string  `json:"receiver_scope"`
	StoreID       *uint64 `json:"store_id"`
	RatioBps      int     `json:"ratio_bps"`
	FixedAmount   int64   `json:"fixed_amount"`
}

type SplitExecutionModel struct {
	OrderNo          string     `json:"order_no"`
	ReceiverEntityID uint64     `json:"receiver_entity_id"`
	ReceiverType     string     `json:"receiver_type"`
	Amount           int64      `json:"amount"`
	Level            int        `json:"level"`
	Status           string     `json:"status"`
	ChannelSplitNo   string     `json:"channel_split_no"`
	LastError        string     `json:"last_error"`
	Attempt          int        `json:"attempt"`
	NextRetryAt      *time.Time `json:"next_retry_at"`
	MerchantID       uint64     `json:"merchant_id"`
	RuleCode         string     `json:"rule_code"`
	RuleSnapshot     string     `json:"rule_snapshot"`
}

func (SplitExecutionModel) TableName() string { return "t_split_execution" }

type ExecuteRequest struct {
	OrderNo     string
	MerchantID  uint64
	Channel     string
	Allocations []Allocation
	PaidAt      string
	RuleCode    string
	RuleID      *uint64
	StoreID     *uint64
}

type Executor struct {
	orderRepo      *splitrepo.SplitOrderStatusRepo
	ruleRepo       *splitrepo.SplitRuleRepo
	diffRepo       *splitrepo.ReconcileDiffRepo
	ledgerSvc      *ledger.Service
	router         *channel.Router
	alerter        notify.Alerter
	walletRepo     *repository.WalletRepo
	revenueQuerier splitrepo.StoreRevenueQuerier
	logger         *zap.Logger
}

func NewExecutor(
	orderRepo *splitrepo.SplitOrderStatusRepo,
	ruleRepo *splitrepo.SplitRuleRepo,
	diffRepo *splitrepo.ReconcileDiffRepo,
	ledgerSvc *ledger.Service,
	router *channel.Router,
	alerter notify.Alerter,
	walletRepo *repository.WalletRepo,
	revenueQuerier splitrepo.StoreRevenueQuerier,
	logger *zap.Logger,
) *Executor {
	return &Executor{
		orderRepo:      orderRepo,
		ruleRepo:       ruleRepo,
		diffRepo:       diffRepo,
		ledgerSvc:      ledgerSvc,
		router:         router,
		alerter:        alerter,
		walletRepo:     walletRepo,
		revenueQuerier: revenueQuerier,
		logger:         logger,
	}
}

func (e *Executor) SetAlerter(a notify.Alerter) { e.alerter = a }

func (e *Executor) alert(ctx context.Context, title, content string) {
	if e.alerter != nil {
		_ = e.alerter.Alert(ctx, title, content)
	}
}

func sumAmounts(allocations []Allocation) int64 {
	var total int64
	for _, a := range allocations {
		total += a.Amount
	}
	return total
}

func hasSuccess(execs []SplitExecutionModel) bool {
	for _, ex := range execs {
		if ex.Status == "SUCCESS" {
			return true
		}
	}
	return false
}

func AllocationsTotal(allocations []Allocation) int64 { return sumAmounts(allocations) }

func channelReqNo(orderNo string, receiverID uint64, seq int) string {
	return fmt.Sprintf("SR-%s-%d-%d", orderNo, receiverID, seq)
}

func executionID(orderNo string, receiverID uint64, level int) string {
	return fmt.Sprintf("EXEC-%s-%d-%d", orderNo, receiverID, level)
}

