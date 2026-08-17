package event

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"go.uber.org/zap"
)

var testLogger = zap.NewNop()

type mockAlerter struct {
	mu      sync.Mutex
	alerts  []string
}

func (m *mockAlerter) Alert(_ context.Context, title, content string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.alerts = append(m.alerts, title+": "+content)
}

func (m *mockAlerter) count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.alerts)
}

func TestSplitOrderExecutedHandler_DeadAlert(t *testing.T) {
	alerter := &mockAlerter{}
	handler := SplitOrderExecutedHandler(testLogger, alerter)

	payload, _ := json.Marshal(SplitOrderExecutedPayload{
		OrderNo:       "ORD001",
		MerchantID:    123,
		Status:        "DEAD",
		SuccessCount:  2,
		ReceiverCount: 5,
		TotalAmount:   10000,
	})
	event := &DomainEvent{
		EventType: TypeSplitOrderExecuted,
		Payload:   payload,
	}
	if err := handler(context.Background(), event); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if alerter.count() != 1 {
		t.Errorf("expected 1 alert, got %d", alerter.count())
	}
}

func TestSplitOrderExecutedHandler_NoAlertOnSuccess(t *testing.T) {
	alerter := &mockAlerter{}
	handler := SplitOrderExecutedHandler(testLogger, alerter)

	payload, _ := json.Marshal(SplitOrderExecutedPayload{
		OrderNo:       "ORD002",
		MerchantID:    123,
		Status:        "SUCCESS",
		SuccessCount:  5,
		ReceiverCount: 5,
		TotalAmount:   10000,
	})
	event := &DomainEvent{
		EventType: TypeSplitOrderExecuted,
		Payload:   payload,
	}
	if err := handler(context.Background(), event); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if alerter.count() != 0 {
		t.Errorf("expected 0 alerts for SUCCESS, got %d", alerter.count())
	}
}

func TestSplitOrderExecutedHandler_InvalidPayload(t *testing.T) {
	alerter := &mockAlerter{}
	handler := SplitOrderExecutedHandler(testLogger, alerter)

	event := &DomainEvent{
		EventType: TypeSplitOrderExecuted,
		Payload:   json.RawMessage(`{invalid}`),
	}
	if err := handler(context.Background(), event); err != nil {
		t.Fatalf("handler should not return error on invalid payload, got: %v", err)
	}
	if alerter.count() != 0 {
		t.Errorf("expected 0 alerts for invalid payload, got %d", alerter.count())
	}
}

func TestReconcileDiffResolvedHandler(t *testing.T) {
	handler := ReconcileDiffResolvedHandler(testLogger)

	payload, _ := json.Marshal(ReconcileDiffResolvedPayload{
		DiffID:     1,
		MerchantID: 123,
		DiffType:   "AMOUNT_MISMATCH",
	})
	event := &DomainEvent{
		EventType: TypeReconcileDiffResolved,
		Payload:   payload,
	}
	if err := handler(context.Background(), event); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSplitBillApprovedHandler(t *testing.T) {
	handler := SplitBillApprovedHandler(testLogger)

	payload, _ := json.Marshal(SplitBillApprovedPayload{
		BatchNo:     "BATCH001",
		MerchantID:  123,
		TotalAmount: 50000,
	})
	event := &DomainEvent{
		EventType: TypeSplitBillApproved,
		Payload:   payload,
	}
	if err := handler(context.Background(), event); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSplitBillRejectedHandler(t *testing.T) {
	handler := SplitBillRejectedHandler(testLogger)

	payload, _ := json.Marshal(SplitBillRejectedPayload{
		BatchNo:     "BATCH001",
		MerchantID:  123,
	})
	event := &DomainEvent{
		EventType: TypeSplitBillRejected,
		Payload:   payload,
	}
	if err := handler(context.Background(), event); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLogHandler(t *testing.T) {
	handler := LogHandler(testLogger)

	payload, _ := json.Marshal(map[string]string{"key": "value"})
	event := &DomainEvent{
		ID:          "evt-001",
		EventType:   "TEST_EVENT",
		AggregateType: "TEST",
		AggregateID: "123",
		Payload:     payload,
	}
	if err := handler(context.Background(), event); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}