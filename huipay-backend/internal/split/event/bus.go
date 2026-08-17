package event

import (
	"context"
	"sync"
)

// Handler 事件处理函数。
type Handler func(ctx context.Context, event *DomainEvent) error

// Bus 内存事件总线（发布-订阅）。
type Bus struct {
	mu       sync.RWMutex
	handlers map[string][]Handler
}

// NewBus 构造 Bus。
func NewBus() *Bus {
	return &Bus{
		handlers: make(map[string][]Handler),
	}
}

// Subscribe 订阅事件类型。
func (b *Bus) Subscribe(eventType string, handler Handler) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers[eventType] = append(b.handlers[eventType], handler)
}

// Publish 同步投递事件到所有订阅者。
// 任一订阅者返回错误，后续仍继续执行，但最终返回首个错误。
func (b *Bus) Publish(ctx context.Context, event *DomainEvent) error {
	b.mu.RLock()
	handlers := b.handlers[event.EventType]
	b.mu.RUnlock()

	if len(handlers) == 0 {
		return nil
	}

	var firstErr error
	for _, h := range handlers {
		if err := h(ctx, event); err != nil {
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}