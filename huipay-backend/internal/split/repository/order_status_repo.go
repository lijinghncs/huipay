// 包 repository 提供分账订单级状态(t_split_order_status)与审计(t_split_audit)数据访问。
package repository

import (
	"context"
	"time"

	"gorm.io/gorm"

	"github.com/huipay/huipay-backend/internal/split/state"
)

// SplitOrderStatusModel 分账订单级状态表 GORM 模型（t_split_order_status）。
type SplitOrderStatusModel struct {
	OrderNo       string     `gorm:"column:order_no;primaryKey"`
	MerchantID    uint64     `gorm:"column:merchant_id;not null"`
	RuleID        *uint64    `gorm:"column:rule_id"`
	RuleSnapshot  string     `gorm:"column:rule_snapshot;type:json"` // 分配快照（重试一致性）
	TotalAmount   int64      `gorm:"column:total_amount;not null"`
	ReceiverCount int        `gorm:"column:receiver_count;not null;default:0"`
	SuccessCount  int        `gorm:"column:success_count;not null;default:0"`
	Status        string     `gorm:"column:status;size:16;not null;default:PENDING"`
	AttemptCount  int        `gorm:"column:attempt_count;not null;default:0"`
	NextRetryAt   *time.Time `gorm:"column:next_retry_at"`
	Degraded      int        `gorm:"column:degraded;not null;default:0"`
	LastError     string     `gorm:"column:last_error;size:512"`
	CreatedAt     time.Time  `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt     time.Time  `gorm:"column:updated_at;autoUpdateTime"`
}

// TableName 表名。
func (SplitOrderStatusModel) TableName() string { return "t_split_order_status" }

// SplitOrderStatusRepo 分账订单级状态仓储。
type SplitOrderStatusRepo struct{ db *gorm.DB }

// NewSplitOrderStatusRepo 构造 SplitOrderStatusRepo。
func NewSplitOrderStatusRepo(db *gorm.DB) *SplitOrderStatusRepo { return &SplitOrderStatusRepo{db: db} }

// DB 暴露底层连接（供服务/调度器复用同一事务连接）。
func (r *SplitOrderStatusRepo) DB() *gorm.DB { return r.db }

// Upsert 创建或更新订单状态；已存在则仅更新可重算字段（不覆盖快照/创建时间）。
func (r *SplitOrderStatusRepo) Upsert(ctx context.Context, m *SplitOrderStatusModel) error {
	return r.db.WithContext(ctx).Save(m).Error
}

// Get 查询订单状态。
func (r *SplitOrderStatusRepo) Get(ctx context.Context, orderNo string) (*SplitOrderStatusModel, error) {
	var m SplitOrderStatusModel
	if err := r.db.WithContext(ctx).Where("order_no = ?", orderNo).First(&m).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &m, nil
}

// MarkProcessing 将订单置为处理中（开始执行前调用）。
func (r *SplitOrderStatusRepo) MarkProcessing(ctx context.Context, orderNo string) error {
	return r.db.WithContext(ctx).Model(&SplitOrderStatusModel{}).
		Where("order_no = ?", orderNo).
		Update("status", state.Processing).Error
}

// UpdateResult 执行结束后回写成果（成功数/状态/错误/下次重试时间）。attemptCount 为本次后的累计值。
func (r *SplitOrderStatusRepo) UpdateResult(ctx context.Context, orderNo string, successCount int, status string, attemptCount int, nextRetryAt *time.Time, lastErr string) error {
	return r.db.WithContext(ctx).Model(&SplitOrderStatusModel{}).
		Where("order_no = ?", orderNo).
		Updates(map[string]any{
			"success_count": successCount,
			"status":        status,
			"attempt_count": attemptCount,
			"next_retry_at": nextRetryAt,
			"last_error":    lastErr,
			"degraded":      gorm.Expr("degraded"), // 保留
		}).Error
}

// ClaimRetry 原子认领可重试订单（并发安全）：仅当状态可重试且尝试次数未耗尽且到达重试时间时置 PROCESSING。
func (r *SplitOrderStatusRepo) ClaimRetry(ctx context.Context, orderNo string, now time.Time) (bool, error) {
	res := r.db.WithContext(ctx).Model(&SplitOrderStatusModel{}).
		Where("order_no = ?", orderNo).
		Where("status IN ?", []string{string(state.Failed), string(state.Partial), string(state.Suspended)}).
		Where("attempt_count < ?", maxRetryAttempts).
		Where("next_retry_at IS NULL OR next_retry_at <= ?", now).
		Updates(map[string]any{"status": state.Processing})
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
}

// MarkSuspended 悬挂检测：处理中超时的订单置为 SUSPENDED。
func (r *SplitOrderStatusRepo) MarkSuspended(ctx context.Context, before time.Time) (int64, error) {
	res := r.db.WithContext(ctx).Model(&SplitOrderStatusModel{}).
		Where("status = ?", state.Processing).
		Where("updated_at < ?", before).
		Update("status", state.Suspended)
	if res.Error != nil {
		return 0, res.Error
	}
	return res.RowsAffected, nil
}

// ListRetryCandidates 列出待补偿订单号（FAILED/PARTIAL/SUSPENDED 且未耗尽且到期）。
func (r *SplitOrderStatusRepo) ListRetryCandidates(ctx context.Context, now time.Time, limit int) ([]string, error) {
	var orderNos []string
	if err := r.db.WithContext(ctx).Model(&SplitOrderStatusModel{}).
		Where("status IN ?", []string{string(state.Failed), string(state.Partial), string(state.Suspended)}).
		Where("attempt_count < ?", maxRetryAttempts).
		Where("next_retry_at IS NULL OR next_retry_at <= ?", now).
		Order("next_retry_at ASC").
		Limit(limit).
		Pluck("order_no", &orderNos).Error; err != nil {
		return nil, err
	}
	return orderNos, nil
}

// MarkDead 重试耗尽置 DEAD。
func (r *SplitOrderStatusRepo) MarkDead(ctx context.Context, orderNo, lastErr string) error {
	return r.db.WithContext(ctx).Model(&SplitOrderStatusModel{}).
		Where("order_no = ?", orderNo).
		Updates(map[string]any{"status": state.Dead, "last_error": lastErr}).Error
}

// SuspendedCount 统计悬挂（PROCESSING 超时）订单数，供指标上报。
func (r *SplitOrderStatusRepo) SuspendedCount(ctx context.Context) (int64, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&SplitOrderStatusModel{}).
		Where("status = ?", state.Processing).
		Where("updated_at < ?", time.Now().Add(-10*time.Minute)).
		Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// ResetAttempt 复位尝试次数（手动重试/充值后重试时可选）。
func (r *SplitOrderStatusRepo) ResetAttempt(ctx context.Context, orderNo string) error {
	return r.db.WithContext(ctx).Model(&SplitOrderStatusModel{}).
		Where("order_no = ?", orderNo).
		Update("attempt_count", 0).Error
}

// Reopen 死单复位重开：DEAD → FAILED 并清零重试计数、立即到期，等待补偿调度重入。
// 乐观锁：仅当前状态为 DEAD 才生效，返回是否实际更新。
func (r *SplitOrderStatusRepo) Reopen(ctx context.Context, orderNo string) (bool, error) {
	now := time.Now()
	res := r.db.WithContext(ctx).Model(&SplitOrderStatusModel{}).
		Where("order_no = ? AND status = ?", orderNo, state.Dead).
		Updates(map[string]any{
			"status":        OrderStatusFailed,
			"attempt_count": 0,
			"next_retry_at": &now,
		})
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
}

// MarkResolved 人工核销：DEAD/FAILED/PARTIAL/SUSPENDED → RESOLVED（差错闭环，不再自动重试）。
// 乐观锁：仅当前状态可核销才生效，返回是否实际更新。
func (r *SplitOrderStatusRepo) MarkResolved(ctx context.Context, orderNo string) (bool, error) {
	res := r.db.WithContext(ctx).Model(&SplitOrderStatusModel{}).
		Where("order_no = ?", orderNo).
		Where("status IN ?", []string{string(state.Dead), string(state.Failed), string(state.Partial), string(state.Suspended)}).
		Update("status", state.Resolved)
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
}

// ListExceptions 差错中心聚合查询：某商户异常订单（FAILED/PARTIAL/SUSPENDED/DEAD/RESOLVED）或降级订单。
// status 非空时按单状态过滤；degraded 非 nil 时按降级标记过滤。按更新时间倒序分页。
func (r *SplitOrderStatusRepo) ListExceptions(ctx context.Context, merchantID uint64, status string, degraded *int, offset, limit int) ([]SplitOrderStatusModel, int64, error) {
	db := r.db.WithContext(ctx).Model(&SplitOrderStatusModel{}).
		Where("merchant_id = ?", merchantID)
	if status != "" {
		db = db.Where("status = ?", status)
	} else {
		db = db.Where("(status IN ? OR degraded = 1)", state.ExceptionStatuses)
	}
	if degraded != nil {
		db = db.Where("degraded = ?", *degraded)
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []SplitOrderStatusModel
	if err := db.Order("updated_at DESC").Offset(offset).Limit(limit).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// ListAllExceptions 管理端跨商户异常订单查询（merchantID 不限制）。status 非空时按单状态过滤。
func (r *SplitOrderStatusRepo) ListAllExceptions(ctx context.Context, status string, degraded *int, offset, limit int) ([]SplitOrderStatusModel, int64, error) {
	db := r.db.WithContext(ctx).Model(&SplitOrderStatusModel{})
	if status != "" {
		db = db.Where("status = ?", status)
	} else {
		db = db.Where("(status IN ? OR degraded = 1)", state.ExceptionStatuses)
	}
	if degraded != nil {
		db = db.Where("degraded = ?", *degraded)
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []SplitOrderStatusModel
	if err := db.Order("updated_at DESC").Offset(offset).Limit(limit).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// MaxRetryAttempts 最大补偿重试次数（文档 B1：5 次）。
const MaxRetryAttempts = 5

// maxRetryAttempts 兼容内部引用。
const maxRetryAttempts = MaxRetryAttempts

// RetryBackoff 返回第 attempt 次重试的间隔（指数退避：30s→1m→2m→4m→8m，封顶）。
func RetryBackoff(attempt int) time.Duration {
	base := 30 * time.Second
	d := base
	for i := 1; i < attempt; i++ {
		d *= 2
		if d > 8*time.Minute {
			d = 8 * time.Minute
			break
		}
	}
	return d
}

// SplitAuditModel 分账审计日志表 GORM 模型（t_split_audit）。
type SplitAuditModel struct {
	ID           uint64    `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	BizType      string    `gorm:"column:biz_type;size:32;not null" json:"biz_type"`
	BizID        string    `gorm:"column:biz_id;size:64;not null" json:"biz_id"`
	Action       string    `gorm:"column:action;size:32;not null" json:"action"`
	OperatorType string    `gorm:"column:operator_type;size:16;not null" json:"operator_type"`
	OperatorID   uint64    `gorm:"column:operator_id;not null;default:0" json:"operator_id"`
	Detail       *string   `gorm:"column:detail;type:json" json:"detail"` // nil 写 NULL（JSON 列不接受空字符串）
	CreatedAt    time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
}

// TableName 表名。
func (SplitAuditModel) TableName() string { return "t_split_audit" }

// 审计业务类型常量。
const (
	AuditBizTypeDailySplit = "DAILY_SPLIT"
	AuditBizTypeSplitBill  = "SPLIT_BILL"
	AuditBizTypeSplitExec  = "SPLIT_EXEC"
	AuditBizTypeReconcileDiff = "RECONCILE_DIFF" // 对账差异核销
)

// 审计动作常量（V2 合并版）。
const (
	AuditActionExecute           = "EXECUTE"
	AuditActionExecuteFailed     = "EXECUTE_FAILED"
	AuditActionReconcilePassed   = "RECONCILE_PASSED"
	AuditActionReconcileFailed   = "RECONCILE_FAILED"
	AuditActionReset             = "RESET"
	AuditActionManualOverride    = "MANUAL_OVERRIDE"
	AuditActionApprove           = "APPROVE"
	AuditActionReject            = "REJECT"
	AuditActionRetry             = "RETRY"
	AuditActionReopen            = "REOPEN"  // 死单复位重开
	AuditActionResolve           = "RESOLVE" // 人工核销（订单/差异通用，biz_type 区分）
)

// 审计操作者类型常量。
const (
	AuditOperatorSystem  = "SYSTEM"
	AuditOperatorMerchant = "MERCHANT"
	AuditOperatorAdmin   = "ADMIN"
)

// SplitAuditRepo 分账审计仓储。
type SplitAuditRepo struct{ db *gorm.DB }

// NewSplitAuditRepo 构造 SplitAuditRepo。
func NewSplitAuditRepo(db *gorm.DB) *SplitAuditRepo {
	return &SplitAuditRepo{db: db}
}

// Append 追加一条审计记录。
func (r *SplitAuditRepo) Append(ctx context.Context, m *SplitAuditModel) error {
	return r.db.WithContext(ctx).Create(m).Error
}

// WriteAction 便捷写入（自动 JSON 序列化 detail；nil detail 写 NULL）。
func (r *SplitAuditRepo) WriteAction(ctx context.Context, bizType, bizID, action, operatorType string, operatorID uint64, detail any) error {
	var detailJSON *string
	if detail != nil {
		b, err := marshalDetail(detail)
		if err != nil {
			return err
		}
		detailJSON = &b
	}
	return r.Append(ctx, &SplitAuditModel{
		BizType:      bizType,
		BizID:        bizID,
		Action:       action,
		OperatorType: operatorType,
		OperatorID:   operatorID,
		Detail:       detailJSON,
	})
}

// List 分页查询审计（biz_type/biz_id 可选过滤）。
func (r *SplitAuditRepo) List(ctx context.Context, bizType, bizID, action string, offset, limit int) ([]SplitAuditModel, int64, error) {
	db := r.db.WithContext(ctx).Model(&SplitAuditModel{})
	if bizType != "" {
		db = db.Where("biz_type = ?", bizType)
	}
	if bizID != "" {
		db = db.Where("biz_id = ?", bizID)
	}
	if action != "" {
		db = db.Where("action = ?", action)
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []SplitAuditModel
	if err := db.Order("created_at DESC").Offset(offset).Limit(limit).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}