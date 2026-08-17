package event

import (
	"context"
	"time"

	"go.uber.org/zap"
)

const pollLimit = 50 // 每轮最多拉取 50 条待处理事件

// Worker 后台轮询 outbox 表并投递事件到内存总线。
type Worker struct {
	repo   *OutboxRepo
	bus    *Bus
	logger *zap.Logger
}

// NewWorker 构造 Worker。
func NewWorker(repo *OutboxRepo, bus *Bus, logger *zap.Logger) *Worker {
	return &Worker{repo: repo, bus: bus, logger: logger}
}

// Start 启动轮询循环（阻塞；应在 goroutine 中运行）。
func (w *Worker) Start(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	w.logger.Info("outbox event worker started", zap.Duration("interval", interval))

	// 启动时立即执行一次
	w.poll(ctx)

	for {
		select {
		case <-ticker.C:
			w.poll(ctx)
		case <-ctx.Done():
			w.logger.Info("outbox event worker stopped")
			return
		}
	}
}

// poll 单轮轮询：拉取待处理事件 → 投递到总线 → 标记结果。
func (w *Worker) poll(ctx context.Context) {
	events, err := w.repo.PollPending(ctx, pollLimit)
	if err != nil {
		w.logger.Error("poll outbox events fail", zap.Error(err))
		return
	}

	for _, evt := range events {
		domainEvt := &DomainEvent{
			ID:            evt.ID,
			AggregateType: evt.AggregateType,
			AggregateID:   evt.AggregateID,
			EventType:     evt.EventType,
			Payload:       evt.Payload,
			Timestamp:     evt.CreatedAt,
		}

		if pubErr := w.bus.Publish(ctx, domainEvt); pubErr != nil {
			w.logger.Warn("dispatch event fail",
				zap.String("event_id", evt.ID),
				zap.String("event_type", evt.EventType),
				zap.Error(pubErr))
			if markErr := w.repo.MarkFailed(ctx, evt.ID, pubErr.Error()); markErr != nil {
				w.logger.Error("mark event failed fail", zap.String("event_id", evt.ID), zap.Error(markErr))
			}
			continue
		}

		if err := w.repo.MarkProcessed(ctx, evt.ID); err != nil {
			w.logger.Error("mark event processed fail", zap.String("event_id", evt.ID), zap.Error(err))
		}
	}
}