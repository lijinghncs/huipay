// 包 service 编排分账业务。
package service

import (
	"context"
	"errors"

	"github.com/huipay/huipay-backend/infra/errs"
	"github.com/huipay/huipay-backend/internal/account/service"
	"github.com/huipay/huipay-backend/internal/domain/vo"
	"github.com/huipay/huipay-backend/internal/split/executor"
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
	account    *service.Service
	logger     *zap.Logger
}

// NewService 构造 Service。
func NewService(re *rule.Engine, ex *executor.Executor, acc *service.Service, logger *zap.Logger) *Service {
	return &Service{ruleEngine: re, executor: ex, account: acc, logger: logger}
}

// Execute 执行分账。
func (s *Service) Execute(ctx context.Context, req *ExecuteRequest) (*ExecuteResponse, error) {
	// 骨架：默认按 5:5 拆分（平台 50% + 商户 50%）
	platform := req.Amount * 5000 / 10000
	merchant := req.Amount - platform

	allocations := []executor.Allocation{
		{Level: 1, EntityID: 1, EntityType: vo.EntityPlatform, Amount: platform},
		{Level: 2, EntityID: req.MerchantID, EntityType: vo.EntityMerchant, Amount: merchant},
	}

	// 源账户（平台备付金内部户，骨架中固定为 1）
	const sourceWallet uint64 = 1

	if err := s.executor.Execute(ctx, &executor.ExecuteRequest{
		OrderNo:         req.OrderNo,
		SourceWallet:    sourceWallet,
		Allocations:     allocations,
		IdempotencyKey:  "split",
		TraceID:         req.TraceID,
	}); err != nil {
		return nil, errs.Wrap(errs.CodeInternalError, "split execute failed", 200, err)
	}
	return &ExecuteResponse{OrderNo: req.OrderNo, Allocations: allocations, RuleCode: req.RuleCode}, nil
}

// Get 查询分账结果（骨架：返回空）。
func (s *Service) Get(ctx context.Context, orderNo string) (*ExecuteResponse, error) {
	if orderNo == "" {
		return nil, errors.New("order_no required")
	}
	return &ExecuteResponse{OrderNo: orderNo, Allocations: nil}, nil
}