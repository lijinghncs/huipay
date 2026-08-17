package executor

import (
	"context"
	"fmt"
	"time"
)

func (e *Executor) ListByOrderNo(ctx context.Context, orderNo string) ([]SplitExecutionModel, error) {
	var rows []SplitExecutionModel
	if err := e.journalRepo.DB().WithContext(ctx).
		Where("order_no = ?", orderNo).
		Order("level ASC").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// SplitExecutionSummary 分账记录列表行（按订单聚合）。
type SplitExecutionSummary struct {
	OrderNo       string     `json:"order_no"`
	MerchantName  string     `json:"merchant_name"`
	RuleID        uint64     `json:"rule_id"`
	RuleName      string     `json:"rule_name"`
	TotalAmount   int64      `json:"total_amount"`   // 分账总额（分）
	ReceiverCount int64      `json:"receiver_count"` // 接收方数
	Status        string     `json:"status"`         // SUCCESS / PARTIAL / FAILED
	Channel       string     `json:"channel"`
	ExecutedAt    *time.Time `json:"executed_at"`
}

// SplitExecutionFilter 分账记录列表过滤条件。
type SplitExecutionFilter struct {
	Status string    // SUCCESS / PARTIAL / FAILED（空表示全部）
	Start  time.Time // 执行时间下限（可选）
	End    time.Time // 执行时间上限（可选）
	RuleID uint64    // 命中规则 ID（可选）
}

// ListByMerchant 按商户分页查询分账记录（JOIN t_order 过滤商户，按订单聚合）。
func (e *Executor) ListByMerchant(ctx context.Context, merchantID uint64, offset, limit int, f SplitExecutionFilter) ([]SplitExecutionSummary, int64, error) {
	db := e.journalRepo.DB().WithContext(ctx)

	where := "(o.merchant_id = ? OR sb.merchant_id = ?)"
	args := []any{merchantID, merchantID}
	if !f.Start.IsZero() {
		where += " AND se.executed_at >= ?"
		args = append(args, f.Start)
	}
	if !f.End.IsZero() {
		where += " AND se.executed_at <= ?"
		args = append(args, f.End)
	}
	if f.RuleID > 0 {
		where += " AND se.rule_id = ?"
		args = append(args, f.RuleID)
	}

	having := ""
	switch f.Status {
	case "SUCCESS":
		having = " HAVING success_count = total_count"
	case "FAILED":
		having = " HAVING failed_count = total_count"
	case "PARTIAL":
		having = " HAVING success_count <> total_count AND failed_count <> total_count"
	}

	// 总数与列表共用同一 WHERE + HAVING，保证状态过滤下 total 与 items 一致
	var total int64
	totalQuery := `SELECT COUNT(*) FROM (
		SELECT se.order_no,
			COUNT(*) AS total_count,
			SUM(CASE WHEN se.status = 'SUCCESS' THEN 1 ELSE 0 END) AS success_count,
			SUM(CASE WHEN se.status = 'FAILED' THEN 1 ELSE 0 END) AS failed_count
		FROM t_split_execution se
		LEFT JOIN t_order o ON o.order_no = se.order_no
		LEFT JOIN t_split_bill sb ON sb.batch_no = se.order_no
		WHERE ` + where + `
		GROUP BY se.order_no` + having + `
	) t`
	if err := db.Raw(totalQuery, args...).Scan(&total).Error; err != nil {
		return nil, 0, err
	}

	type row struct {
		OrderNo       string
		MerchantName  string
		RuleID        uint64
		RuleName      string
		TotalAmount   int64
		TotalCount    int64
		SuccessCount  int64
		FailedCount   int64
		Channel       string
		ExecutedAt    *time.Time
	}
	var rows []row
	query := `SELECT
			se.order_no,
			en.name AS merchant_name,
			COALESCE(MAX(se.rule_id), 0) AS rule_id,
			COALESCE(MAX(sr.rule_name), '') AS rule_name,
			COALESCE(SUM(se.amount), 0) AS total_amount,
			COUNT(*) AS total_count,
			SUM(CASE WHEN se.status = 'SUCCESS' THEN 1 ELSE 0 END) AS success_count,
			SUM(CASE WHEN se.status = 'FAILED' THEN 1 ELSE 0 END) AS failed_count,
			MAX(se.channel) AS channel,
			MAX(se.executed_at) AS executed_at
		FROM t_split_execution se
		LEFT JOIN t_order o ON o.order_no = se.order_no
		LEFT JOIN t_split_bill sb ON sb.batch_no = se.order_no
		LEFT JOIN t_entity en ON en.id = COALESCE(o.merchant_id, sb.merchant_id)
		LEFT JOIN t_split_rule sr ON sr.id = se.rule_id
		WHERE ` + where + `
		GROUP BY se.order_no, en.name` + having + `
		ORDER BY executed_at DESC
		LIMIT ? OFFSET ?`
	allArgs := append(args, limit, offset)
	if err := db.Raw(query, allArgs...).Scan(&rows).Error; err != nil {
		return nil, 0, err
	}

	out := make([]SplitExecutionSummary, 0, len(rows))
	for _, r := range rows {
		status := "SUCCESS"
		switch {
		case r.TotalCount > 0 && r.FailedCount == r.TotalCount:
			status = "FAILED"
		case r.SuccessCount != r.TotalCount:
			status = "PARTIAL"
		}
		out = append(out, SplitExecutionSummary{
			OrderNo:       r.OrderNo,
			MerchantName:  r.MerchantName,
			RuleID:        r.RuleID,
			RuleName:      r.RuleName,
			TotalAmount:   r.TotalAmount,
			ReceiverCount: r.TotalCount,
			Status:        status,
			Channel:       r.Channel,
			ExecutedAt:    r.ExecutedAt,
		})
	}
	return out, total, nil
}

// ListByOrderNoForMerchant 按订单查询分账执行记录，并校验归属商户（orderNo 可为真实订单号或分账单批次号；nil,nil 表示非本商户）。
func (e *Executor) ListByOrderNoForMerchant(ctx context.Context, merchantID uint64, orderNo string) ([]SplitExecutionModel, error) {
	var owner int64
	if err := e.journalRepo.DB().WithContext(ctx).Raw(
		`SELECT
			(SELECT COUNT(*) FROM t_order WHERE order_no = ? AND merchant_id = ?)
			+ (SELECT COUNT(*) FROM t_split_bill WHERE batch_no = ? AND merchant_id = ?)`,
		orderNo, merchantID, orderNo, merchantID,
	).Scan(&owner).Error; err != nil {
		return nil, err
	}
	if owner == 0 {
		return nil, nil
	}
	return e.ListByOrderNo(ctx, orderNo)
}

// SplitExecutionDetail 分账明细行（含接收方名称）。
type SplitExecutionDetail struct {
	ReceiverEntityID uint64     `json:"receiver_entity_id"`
	ReceiverType     string     `json:"receiver_type"`
	ReceiverName     string     `json:"receiver_name"`
	Amount           int64      `json:"amount"`
	Level            int        `json:"level"`
	Status           string     `json:"status"`
	ChannelSplitNo   string     `json:"channel_split_no"`
	RetryCount       int        `json:"retry_count"`
	LastError        string     `json:"last_error"`
	ExecutedAt       *time.Time `json:"executed_at"`
}

// ListByOrderNoWithReceiver 按订单查询分账明细并回填接收方名称（校验订单归属商户；orderNo 可为真实订单号或分账单批次号）。
// 返回 nil,nil 表示订单/批次不存在或不属于该商户。
func (e *Executor) ListByOrderNoWithReceiver(ctx context.Context, merchantID uint64, orderNo string) ([]SplitExecutionDetail, error) {
	db := e.journalRepo.DB().WithContext(ctx)

	var owner int64
	if err := db.Raw(
		`SELECT
			(SELECT COUNT(*) FROM t_order WHERE order_no = ? AND merchant_id = ?)
			+ (SELECT COUNT(*) FROM t_split_bill WHERE batch_no = ? AND merchant_id = ?)`,
		orderNo, merchantID, orderNo, merchantID,
	).Scan(&owner).Error; err != nil {
		return nil, err
	}
	if owner == 0 {
		return nil, nil
	}

	type row struct {
		ReceiverEntityID uint64
		ReceiverType     string
		Amount           int64
		Level            int
		Status           string
		ChannelSplitNo   string
		RetryCount       int
		LastError        string
		ExecutedAt       *time.Time
		StoreName        string // STORE 接收方名称
		MerchantName     string // MERCHANT 接收方名称
	}
	var rows []row
	if err := db.Raw(`SELECT
			se.receiver_entity_id,
			se.receiver_type,
			se.amount,
			se.level,
			se.status,
			se.channel_split_no,
			se.retry_count,
			se.last_error,
			se.executed_at,
			st.name AS store_name,
			e.name AS merchant_name
		FROM t_split_execution se
		LEFT JOIN t_store st ON se.receiver_type = 'STORE' AND st.id = se.receiver_entity_id
		LEFT JOIN t_entity e ON se.receiver_type = 'MERCHANT' AND e.id = se.receiver_entity_id
		WHERE se.order_no = ?
		ORDER BY se.level ASC`, orderNo).Scan(&rows).Error; err != nil {
		return nil, err
	}

	out := make([]SplitExecutionDetail, 0, len(rows))
	for _, r := range rows {
		name := fmt.Sprintf("#%d", r.ReceiverEntityID)
		if r.ReceiverType == "STORE" && r.StoreName != "" {
			name = r.StoreName
		} else if r.ReceiverType == "MERCHANT" && r.MerchantName != "" {
			name = r.MerchantName
		}
		out = append(out, SplitExecutionDetail{
			ReceiverEntityID: r.ReceiverEntityID,
			ReceiverType:     r.ReceiverType,
			ReceiverName:     name,
			Amount:           r.Amount,
			Level:            r.Level,
			Status:           r.Status,
			ChannelSplitNo:   r.ChannelSplitNo,
			RetryCount:       r.RetryCount,
			LastError:        r.LastError,
			ExecutedAt:       r.ExecutedAt,
		})
	}
	return out, nil
}

func (e *Executor) sumAmounts(items []Allocation) (int64, error) {
	var t int64
	for _, a := range items {
		if a.Amount <= 0 {
			return 0, fmt.Errorf("invalid allocation: level=%d entity=%d amount=%d", a.Level, a.EntityID, a.Amount)
		}
		t += a.Amount
	}
	return t, nil
}

// hasSuccess 判断某订单的某接收方是否已成功分账。
func (e *Executor) hasSuccess(ctx context.Context, orderNo string, entityID uint64) (bool, error) {
	var count int64
	err := e.journalRepo.DB().WithContext(ctx).
		Model(&SplitExecutionModel{}).
		Where("order_no = ? AND receiver_entity_id = ? AND status = ?", orderNo, entityID, "SUCCESS").
		Count(&count).Error
	return count > 0, err
}
