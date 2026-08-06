// 包 outbox 实现本地消息表事件 worker，替代 Kafka。
// 事务内写入 t_outbox_event，worker 轮询执行；保证至少一次投递。
package outbox

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"gorm.io/gorm"
	"go.uber.org/zap"

	"github.com/huipay/huipay-backend/infra/obs"
)

// Event Outbox 事件。
type Event struct {
	ID           string    `gorm:"column:id;primaryKey;size:20"`
	AggregateType string   `gorm:"column:aggregate_type;size:64"`
	AggregateID   string   `gorm:"column:aggregate_id;size:64"`
	EventType     string   `gorm:"column:event_type;size:64"`
	Payload       string   `gorm:"column:payload;type:json"`
	Status        string   `gorm:"column:status;size:16"`
	RetryCount    int      `gorm:"column:retry_count"`
	NextRetryAt   *time.Time `gorm:"column:next_retry_at"`
	ProcessedAt   *time.Time `gorm:"column:processed_at"`
	CreatedAt     time.Time `gorm:"column:created_at"`
}

// TableName 表名。
func (Event) TableName() string { return "t_outbox_event" }

// Handler 事件处理函数。
type Handler func(ctx context.Context, e *Event) error

// Worker 轮询 worker。
type Worker struct {
	db       *gorm.DB
	logger   *zap.Logger
	handlers map[string]Handler
	interval time.Duration
	wg       sync.WaitGroup
	stop     chan struct{}
}

// NewWorker 构造 Worker。
func NewWorker(db *gorm.DB, logger *zap.Logger) *Worker {
	return &Worker{
		db: db, logger: logger,
		handlers: make(map[string]Handler),
		interval: 1 * time.Second,
		stop:     make(chan struct{}),
	}
}

// Register 注册事件处理器。
func (w *Worker) Register(eventType string, h Handler) { w.handlers[eventType] = h }

// Start 启动 worker（阻塞直到 ctx 取消）。
func (w *Worker) Start(ctx context.Context) {
	t := time.NewTicker(w.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			w.wg.Wait()
			return
		case <-t.C:
			w.tick(ctx)
		}
	}
}

func (w *Worker) tick(ctx context.Context) {
	var batch []Event
	now := time.Now()
	if err := w.db.WithContext(ctx).
		Where("status = ? AND (next_retry_at IS NULL OR next_retry_at <= ?)", "PENDING", now).
		Limit(100).
		Find(&batch).Error; err != nil {
		w.logger.Error("outbox fetch failed", zap.Error(err))
		return
	}
	for i := range batch {
		e := batch[i]
		h, ok := w.handlers[e.EventType]
		if !ok {
			w.logger.Warn("no handler", zap.String("type", e.EventType))
			continue
		}
		w.wg.Add(1)
		go func(ev Event) {
			defer w.wg.Done()
			if err := h(ctx, &ev); err != nil {
				w.requeue(&ev, err)
				return
			}
			w.markDone(&ev)
		}(e)
	}
}

func (w *Worker) markDone(e *Event) {
	now := time.Now()
	w.db.Model(e).Updates(map[string]any{"status": "PROCESSED", "processed_at": now})
}

func (w *Worker) requeue(e *Event, err error) {
	e.RetryCount++
	backoff := time.Duration(1<<e.RetryCount) * time.Second
	if backoff > 5*time.Minute {
		backoff = 5 * time.Minute
	}
	next := time.Now().Add(backoff)
	status := "PENDING"
	if e.RetryCount >= 5 {
		status = "FAILED"
	}
	w.db.Model(e).Updates(map[string]any{
		"status": status, "retry_count": e.RetryCount, "next_retry_at": next,
	})
	w.logger.Warn("outbox handler failed",
		zap.String("event_id", e.ID), zap.Error(err),
		zap.Int("retry", e.RetryCount),
	)
}

// Append 在事务内写入 outbox 事件。
func Append(db *gorm.DB, aggregateType, aggregateID, eventType string, payload any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	e := &Event{
		AggregateType: aggregateType,
		AggregateID:   aggregateID,
		EventType:     eventType,
		Payload:       string(raw),
		Status:        "PENDING",
	}
	return db.Create(e).Error
}

var _ = obs.TraceIDKey // 防止未使用（保留 trace 链路扩展点）