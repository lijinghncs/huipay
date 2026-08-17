package executor

import (
	"testing"

	"github.com/huipay/huipay-backend/internal/split/state"
)

func TestDetermineFinalStatus_Success(t *testing.T) {
	status, nextRetryAt := DetermineFinalStatus("", 5, 5, 1)
	if status != state.Success {
		t.Errorf("status = %v, want SUCCESS", status)
	}
	if nextRetryAt != nil {
		t.Error("nextRetryAt should be nil for success")
	}
}

func TestDetermineFinalStatus_Partial(t *testing.T) {
	status, nextRetryAt := DetermineFinalStatus("timeout", 3, 5, 1)
	if status != state.Partial {
		t.Errorf("status = %v, want PARTIAL", status)
	}
	if nextRetryAt == nil {
		t.Error("nextRetryAt should not be nil for partial")
	}
}

func TestDetermineFinalStatus_Failed(t *testing.T) {
	status, nextRetryAt := DetermineFinalStatus("channel error", 0, 5, 1)
	if status != state.Failed {
		t.Errorf("status = %v, want FAILED", status)
	}
	if nextRetryAt == nil {
		t.Error("nextRetryAt should not be nil for failed")
	}
}

func TestDetermineFinalStatus_Dead(t *testing.T) {
	status, nextRetryAt := DetermineFinalStatus("channel error", 0, 5, 100)
	if status != state.Dead {
		t.Errorf("status = %v, want DEAD", status)
	}
	if nextRetryAt != nil {
		t.Error("nextRetryAt should be nil for dead")
	}
}

func TestDetermineFinalStatus_PartialToDead(t *testing.T) {
	status, _ := DetermineFinalStatus("timeout", 3, 5, 100)
	if status != state.Dead {
		t.Errorf("status = %v, want DEAD (partial exhausted)", status)
	}
}

func TestDetermineFinalStatus_AllFailed(t *testing.T) {
	status, nextRetryAt := DetermineFinalStatus("network error", 0, 3, 1)
	if status != state.Failed {
		t.Errorf("status = %v, want FAILED", status)
	}
	if nextRetryAt == nil {
		t.Error("nextRetryAt should not be nil")
	}
}

func TestDetermineFinalStatus_ZeroReceiverCount(t *testing.T) {
	// 当 receiverCount = 0 时，应默认为 1 避免除零
	status, _ := DetermineFinalStatus("error", 0, 0, 1)
	if status != state.Failed {
		t.Errorf("status = %v, want FAILED", status)
	}
}

func TestDetermineFinalStatus_SuccessWithAttempts(t *testing.T) {
	// 即使尝试次数很高，成功后仍为 Success
	status, nextRetryAt := DetermineFinalStatus("", 5, 5, 100)
	if status != state.Success {
		t.Errorf("status = %v, want SUCCESS", status)
	}
	if nextRetryAt != nil {
		t.Error("nextRetryAt should be nil for success")
	}
}

func TestDetermineFinalStatus_ZeroAttempt(t *testing.T) {
	status, nextRetryAt := DetermineFinalStatus("error", 0, 1, 0)
	if status != state.Failed {
		t.Errorf("status = %v, want FAILED", status)
	}
	if nextRetryAt == nil {
		t.Error("nextRetryAt should not be nil for attempt=0 (defaults to 1)")
	}
}