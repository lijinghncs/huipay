// 包 service 编排分账业务。
package service

import (
	"context"
	"errors"
	"time"

	"github.com/huipay/huipay-backend/infra/errs"
	"github.com/huipay/huipay-backend/internal/account/service"
	"github.com/huipay/huipay-backend/internal/domain/vo"
	"github.com/huipay/huipay-backend/internal/split/executor"
	"github.com/huipay/huipay-backend/internal/split/repository"
	"github.com/huipay/huipay-backend/internal/split/rule"
	"go.uber.org/zap"
)

// ExecuteRequest 分账执行请求（HTTP 层）。
type ExecuteRequest struct {
	OrderNo    string             `json:"order_no" binding:"required"`
	MerchantID uint64             `json:"merchant_id" binding:"required"`
	Amount     int64              `json:"amount" binding:"required,gt=0"`
	RuleCode   string             `json:"rule_code"` // 可选：指定规则
	StoreID    uint64             `json:"store_id"`
	Channel    vo.ChannelCode     `json:"channel"`
	TraceID    string             `json:"-"`
}

// ExecuteResponse 分账执行响应。
type ExecuteResponse struct {
	OrderNo    string                       `json:"order_no"`
	Allocations []executor.Allocation       `json:"allocations"`
	RuleCode   string                       `json:"rule_code"`
}

// Service 分账服务。
type Service struct {
	ruleEngine *rule.Engine
	executor   *executor.Executor
	ruleRepo   *repository.SplitRuleRepo
	account    *service.Service
	logger     *zap.Logger
}

// NewService 构造 Service。
func NewService(re *rule.Engine, ex *executor.Executor, ruleRepo *repository.SplitRuleRepo, acc *service.Service, logger *zap.Logger) *Service {
	return &Service{ruleEngine: re, executor: ex, ruleRepo: ruleRepo, account: acc, logger: logger}
}

// Execute 执行分账：按规则引擎匹配（支持门店维度），解析分配方案后落地账本。
func (s *Service) Execute(ctx context.Context, req *ExecuteRequest) (*ExecuteResponse, error) {
	if s.ruleRepo == nil {
		return nil, errs.New(errs.CodeInternalError, "split rule repo not configured", 500)
	}
	// 1) 确认具备规则引擎与执行器
	if s.ruleEngine == nil || s.executor == nil {
		return nil, errs.New(errs.CodeInternalError, "split engine not configured", 500)
	}

	// 2) 匹配规则（指定 RuleCode 直选，否则按宿主商户 + 门店 + 通道匹配）
	var matched *rule.Rule
	var err error
	if req.RuleCode != "" {
		matched, err = s.ruleRepo.GetByCodeAndMerchant(ctx, req.RuleCode, req.MerchantID)
		if err != nil {
			return nil, errs.Wrap(errs.CodeInternalError, "query split rule failed", 200, err)
		}
		if matched == nil {
			return nil, errs.New(errs.CodeSplitRuleNotMatch, "split rule not found", 200)
		}
	} else {
		rules, rErr := s.ruleRepo.ListByMerchant(ctx, req.MerchantID)
		if rErr != nil {
			return nil, errs.Wrap(errs.CodeInternalError, "load split rules failed", 200, rErr)
		}
		matched = s.ruleEngine.Resolve(rules, rule.MatchContext{
			MerchantID: req.MerchantID,
			Channel:    string(req.Channel),
			StoreID:    req.StoreID,
			NowAt:      time.Now().Format(time.RFC3339),
		})
		if matched == nil {
			return nil, errs.New(errs.CodeSplitRuleNotMatch, "no matching split rule", 200)
		}
	}

	// 3) 由规则分配方案计算金额并映射为执行单元
	allocations, aErr := s.buildAllocations(ctx, matched, req.Amount)
	if aErr != nil {
		return nil, aErr
	}

	// 4) 源账户（平台备付金内部户，骨架中固定为 1）
	const sourceWallet uint64 = 1

	// 5) 执行落地
	if err := s.executor.Execute(ctx, &executor.ExecuteRequest{
		OrderNo:       req.OrderNo,
		SourceWallet:  sourceWallet,
		Allocations:   allocations,
		StoreID:       req.StoreID,
		IdempotencyKey: "split",
		TraceID:       req.TraceID,
	}); err != nil {
		return nil, errs.Wrap(errs.CodeInternalError, "split execute failed", 200, err)
	}
	return &ExecuteResponse{OrderNo: req.OrderNo, Allocations: allocations, RuleCode: matched.RuleCode}, nil
}

// buildAllocations 将规则分配方案（比例/固定额）换算为金额执行单元。
func (s *Service) buildAllocations(ctx context.Context, r *rule.Rule, total int64) ([]executor.Allocation, error) {
	if len(r.Allocations) == 0 {
		return nil, errs.New(errs.CodeSplitRuleNotMatch, "split rule has no allocations", 200)
	}
	allocations := make([]executor.Allocation, 0, len(r.Allocations))
	var used int64
	for i, a := range r.Allocations {
		amount := a.FixedAmount
		if a.RatioBps > 0 {
			amount = total * a.RatioBps / 10000
		}
		if amount <= 0 {
			return nil, errs.New(errs.CodeInvalidParams, "invalid allocation amount", 200)
		}
		// 最后一笔按剩余补齐，避免比例取整丢分
		if i == len(r.Allocations)-1 {
			remain := total - used
			if remain > 0 && remain != amount {
				amount = remain
			}
		}
		used += amount
		allocations = append(allocations, executor.Allocation{
			Level:      i + 1,
			EntityID:   a.ReceiverEntityID,
			EntityType: vo.EntityType(a.ReceiverType),
			Amount:     amount,
		})
	}
	if used > total {
		return nil, errs.New(errs.CodeInvalidParams, "allocations exceed order amount", 200)
	}
	_ = ctx
	return allocations, nil
}

// Get 查询分账结果（骨架：返回空）。
func (s *Service) Get(ctx context.Context, orderNo string) (*ExecuteResponse, error) {
	if orderNo == "" {
		return nil, errors.New("order_no required")
	}
	return &ExecuteResponse{OrderNo: orderNo, Allocations: nil}, nil
}