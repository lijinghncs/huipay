package event

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestBus_SubscribeAndPublish(t *testing.T) {
	bus := NewBus()
	ctx := context.Background()

	var called atomic.Int32
	bus.Subscribe("TEST_EVENT", func(_ context.Context, e *DomainEvent) error {
		called.Add(1)
		return nil
	})

	err := bus.Publish(ctx, &DomainEvent{
		EventType: "TEST_EVENT",
		Timestamp: time.Now(),
	})
	if err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
	if called.Load() != 1 {
		t.Errorf("handler called %d times, want 1", called.Load())
	}
}

func TestBus_MultipleSubscribers(t *testing.T) {
	bus := NewBus()
	ctx := context.Background()

	var c1, c2 atomic.Int32
	bus.Subscribe("MULTI", func(_ context.Context, e *DomainEvent) error {
		c1.Add(1)
		return nil
	})
	bus.Subscribe("MULTI", func(_ context.Context, e *DomainEvent) error {
		c2.Add(1)
		return nil
	})

	_ = bus.Publish(ctx, &DomainEvent{EventType: "MULTI"})
	if c1.Load() != 1 || c2.Load() != 1 {
		t.Errorf("handler counts: c1=%d c2=%d, want both 1", c1.Load(), c2.Load())
	}
}

func TestBus_NoSubscriber(t *testing.T) {
	bus := NewBus()
	err := bus.Publish(context.Background(), &DomainEvent{EventType: "NOBODY"})
	if err != nil {
		t.Errorf("Publish with no subscriber should not error, got %v", err)
	}
}

func TestBus_ErrorPropagation(t *testing.T) {
	bus := NewBus()
	ctx := context.Background()

	wantErr := errors.New("handler error")
	bus.Subscribe("ERR", func(_ context.Context, e *DomainEvent) error {
		return wantErr
	})

	err := bus.Publish(ctx, &DomainEvent{EventType: "ERR"})
	if err == nil {
		t.Fatal("Publish() expected error")
	}
}

func TestBus_MultipleSubscribersOneError(t *testing.T) {
	bus := NewBus()
	ctx := context.Background()

	var c2 atomic.Int32
	bus.Subscribe("MIX", func(_ context.Context, e *DomainEvent) error {
		return errors.New("oops")
	})
	bus.Subscribe("MIX", func(_ context.Context, e *DomainEvent) error {
		c2.Add(1)
		return nil
	})

	err := bus.Publish(ctx, &DomainEvent{EventType: "MIX"})
	if err == nil {
		t.Fatal("Publish() expected error")
	}
	// 第二个 handler 即使第一个报错也应执行
	if c2.Load() != 1 {
		t.Errorf("second handler called %d times, want 1", c2.Load())
	}
}

func TestBus_WrongEventType(t *testing.T) {
	bus := NewBus()
	ctx := context.Background()

	var called atomic.Int32
	bus.Subscribe("TYPE_A", func(_ context.Context, e *DomainEvent) error {
		called.Add(1)
		return nil
	})

	_ = bus.Publish(ctx, &DomainEvent{EventType: "TYPE_B"})
	if called.Load() != 0 {
		t.Error("handler for TYPE_A should not be called for TYPE_B event")
	}
}

func TestBus_DifferentEventTypes(t *testing.T) {
	bus := NewBus()
	ctx := context.Background()

	var a, b atomic.Int32
	bus.Subscribe("A", func(_ context.Context, e *DomainEvent) error {
		a.Add(1)
		return nil
	})
	bus.Subscribe("B", func(_ context.Context, e *DomainEvent) error {
		b.Add(1)
		return nil
	})

	_ = bus.Publish(ctx, &DomainEvent{EventType: "A"})
	_ = bus.Publish(ctx, &DomainEvent{EventType: "B"})
	_ = bus.Publish(ctx, &DomainEvent{EventType: "A"})

	if a.Load() != 2 {
		t.Errorf("A handler called %d times, want 2", a.Load())
	}
	if b.Load() != 1 {
		t.Errorf("B handler called %d times, want 1", b.Load())
	}
}

func TestBus_PublishPayload(t *testing.T) {
	bus := NewBus()
	ctx := context.Background()

	var gotPayload string
	bus.Subscribe("PAYLOAD", func(_ context.Context, e *DomainEvent) error {
		gotPayload = string(e.Payload)
		return nil
	})

	_ = bus.Publish(ctx, &DomainEvent{
		EventType: "PAYLOAD",
		Payload:   []byte(`{"key":"value"}`),
	})
	if gotPayload != `{"key":"value"}` {
		t.Errorf("payload = %s, want %s", gotPayload, `{"key":"value"}`)
	}
}