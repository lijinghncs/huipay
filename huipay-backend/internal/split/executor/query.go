package executor

import (
	"context"
	"time"

	splitrepo "github.com/huipay/huipay-backend/internal/split/repository"
	"github.com/huipay/huipay-backend/internal/split/splitcfg"
)

func (e *Executor) ListByOrderNo(ctx context.Context, orderNo string) ([]SplitExecutionModel, error) {
	models, err := e.orderRepo.ListByOrderNo(ctx, orderNo)
	if err != nil {
		return nil, err
	}
	result := make([]SplitExecutionModel, 0, len(models))
	for _, m := range models {
		result = append(result, SplitExecutionModel{
			OrderNo:          m.OrderNo,
			ReceiverEntityID: m.ReceiverEntityID,
			ReceiverType:     m.ReceiverType,
			Amount:           m.Amount,
			Level:            m.Level,
			Status:           m.Status,
			ChannelSplitNo:   m.ChannelSplitNo,
			LastError:        m.LastError,
			Attempt:          m.Attempt,
			NextRetryAt:      m.NextRetryAt,
			MerchantID:       m.MerchantID,
			RuleCode:         m.RuleCode,
			RuleSnapshot:     m.RuleSnapshot,
		})
	}
	return result, nil
}

func (e *Executor) ListByOrderNoForMerchant(ctx context.Context, merchantID uint64, orderNo string) ([]SplitExecutionModel, error) {
	models, err := e.orderRepo.ListByOrderNoForMerchant(ctx, merchantID, orderNo)
	if err != nil {
		return nil, err
	}
	result := make([]SplitExecutionModel, 0, len(models))
	for _, m := range models {
		result = append(result, SplitExecutionModel{
			OrderNo:          m.OrderNo,
			ReceiverEntityID: m.ReceiverEntityID,
			ReceiverType:     m.ReceiverType,
			Amount:           m.Amount,
			Level:            m.Level,
			Status:           m.Status,
			ChannelSplitNo:   m.ChannelSplitNo,
			LastError:        m.LastError,
			Attempt:          m.Attempt,
			NextRetryAt:      m.NextRetryAt,
			MerchantID:       m.MerchantID,
			RuleCode:         m.RuleCode,
			RuleSnapshot:     m.RuleSnapshot,
		})
	}
	return result, nil
}

func (e *Executor) ListByOrderNoWithReceiver(ctx context.Context, merchantID uint64, orderNo string) ([]SplitExecutionDetail, error) {
	models, err := e.orderRepo.ListByOrderNoForMerchant(ctx, merchantID, orderNo)
	if err != nil {
		return nil, err
	}
	result := make([]SplitExecutionDetail, 0, len(models))
	for _, m := range models {
		_ = e.orderRepo // keep reference
		result = append(result, SplitExecutionDetail{
			ReceiverEntityID: m.ReceiverEntityID,
			ReceiverType:     m.ReceiverType,
			Amount:           m.Amount,
			Level:            m.Level,
			Status:           m.Status,
			LastError:        m.LastError,
		})
	}
	return result, nil
}

func (e *Executor) ListByMerchant(ctx context.Context, merchantID uint64, offset, limit int, f SplitExecutionFilter) ([]SplitExecutionSummary, int64, error) {
	scope := splitrepo.Scope{MerchantID: merchantID}
	if len(f.Statuses) > 0 {
		scope.Statuses = f.Statuses
	}
	if f.StoreID > 0 {
		scope.StoreID = &f.StoreID
	}
	models, total, err := e.orderRepo.ListByMerchant(ctx, merchantID, scope)
	if err != nil {
		return nil, 0, err
	}
	start := offset
	if start >= len(models) {
		return nil, total, nil
	}
	end := start + limit
	if end > len(models) {
		end = len(models)
	}
	models = models[start:end]
	result := make([]SplitExecutionSummary, 0, len(models))
	for _, m := range models {
		result = append(result, SplitExecutionSummary{
			OrderNo:    m.OrderNo,
			StoreID:    m.StoreID,
			MerchantID: m.MerchantID,
			Total:      m.Amount,
			Status:     m.Status,
			CreatedAt:  m.CreatedAt.Format(time.RFC3339),
		})
	}
	return result, total, nil
}

type SplitExecutionSummary struct {
	OrderNo    string  `json:"order_no"`
	StoreID    *uint64 `json:"store_id"`
	MerchantID uint64  `json:"merchant_id"`
	Total      int64   `json:"total"`
	Status     string  `json:"status"`
	CreatedAt  string  `json:"created_at"`
}

type SplitExecutionDetail struct {
	ReceiverEntityID uint64 `json:"receiver_entity_id"`
	ReceiverType     string `json:"receiver_type"`
	Amount           int64  `json:"amount"`
	Level            int    `json:"level"`
	Status           string `json:"status"`
	LastError        string `json:"last_error"`
}

type SplitExecutionFilter struct {
	Statuses []string
	StoreID  uint64
}
