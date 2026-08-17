package event

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/huipay/huipay-backend/infra/errs"
)

// OutboxEventModel t_outbox_event 表模型。
type OutboxEventModel struct {
	ID            string          `gorm:"column:id;primaryKey;size:20"`
	AggregateType string          `gorm:"column:aggregate_type;size:64;not null"`
	AggregateID   string          `gorm:"column:aggregate_id;size:64;not null"`
	EventType     string          `gorm:"column:event_type;size:64;not null"`
	Payload       json.RawMessage `gorm:"column:payload;type:json;not null"`
	Status        string          `gorm:"column:status;size:16;not null;default:PENDING"`
	RetryCount    int             `gorm:"column:retry_count;not null;default:0"`
	NextRetryAt   *time.Time      `gorm:"column:next_retry_at"`
	ProcessedAt   *time.Time      `gorm:"column:processed_at"`
	CreatedAt     time.Time       `gorm:"column:created_at;not null;default:CURRENT_TIMESTAMP(3)"`
}

// TableName 表名。
func (OutboxEventModel) TableName() string { return "t_outbox_event" }

// OutboxRepo 本地消息表仓储。
type OutboxRepo struct {
	db *gorm.DB
}

// NewOutboxRepo 构造 OutboxRepo。
func NewOutboxRepo(db *gorm.DB) *OutboxRepo {
	return &OutboxRepo{db: db}
}

// Insert 写入事件（幂等：同 ID 已存在则跳过）。
func (r *OutboxRepo) Insert(ctx context.Context, event *DomainEvent) error {
	m := &OutboxEventModel{
		ID:            event.ID,
		AggregateType: event.AggregateType,
		AggregateID:   event.AggregateID,
		EventType:     event.EventType,
		Payload:       event.Payload,
		Status:        "PENDING",
		CreatedAt:     event.Timestamp,
	}
	return r.db.WithContext(ctx).
		Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "id"}}, DoNothing: true}).
		Create(m).Error
}

// PollPending 轮询待处理事件（状态为 PENDING 或 (FAILED AND next_retry_at <= now)）。
func (r *OutboxRepo) PollPending(ctx context.Context, limit int) ([]OutboxEventModel, error) {
	var events []OutboxEventModel
	err := r.db.WithContext(ctx).
		Where("status = ? OR (status = ? AND next_retry_at <= ?)", "PENDING", "FAILED", time.Now()).
		Order("created_at ASC").
		Limit(limit).
		Find(&events).Error
	if err != nil {
		return nil, err
	}
	return events, nil
}

// MarkProcessed 标记事件已处理。
func (r *OutboxRepo) MarkProcessed(ctx context.Context, id string) error {
	now := time.Now()
	return r.db.WithContext(ctx).
		Model(&OutboxEventModel{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"status":       "PROCESSED",
			"processed_at": now,
		}).Error
}

// MarkFailed 标记事件失败（重试计数 + 退避）。
func (r *OutboxRepo) MarkFailed(ctx context.Context, id string, errMsg string) error {
	now := time.Now()
	return r.db.WithContext(ctx).
		Model(&OutboxEventModel{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"status":       "FAILED",
			"retry_count":  gorm.Expr("retry_count + 1"),
			"next_retry_at": now.Add(30 * time.Second), // 固定退避 30s
		}).Error
}

// DeleteProcessed 清理已处理事件（保留最近 7 天）。
func (r *OutboxRepo) DeleteProcessed(ctx context.Context, before time.Time) error {
	return r.db.WithContext(ctx).
		Where("status = ? AND processed_at < ?", "PROCESSED", before).
		Delete(&OutboxEventModel{}).Error
}

// ---------- 全局计数器（事件 ID 生成器）----------

var eventIDCounter atomic.Uint64

// nextEventID 生成唯一事件 ID（格式：前缀+时间戳+序号，共 20 字符）。
func nextEventID() string {
	n := eventIDCounter.Add(1)
	ts := time.Now().UnixMilli()
	return fmt.Sprintf("EVT%d%06d", ts, n%1000000)
}

// ---------- 事件发布辅助 ----------

// PublishEvent 构造领域事件并写入 outbox。
func (r *OutboxRepo) PublishEvent(ctx context.Context, aggType, aggID, eventType string, payload any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return errs.Wrap(errs.CodeInternalError, "marshal event payload failed", 500, err)
	}
	evt := &DomainEvent{
		ID:            nextEventID(),
		AggregateType: aggType,
		AggregateID:   aggID,
		EventType:     eventType,
		Payload:       raw,
		Timestamp:     time.Now(),
	}
	return r.Insert(ctx, evt)
}