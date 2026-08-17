package state

import (
	"testing"
)

func TestStatus_String(t *testing.T) {
	tests := []struct {
		s    Status
		want string
	}{
		{Pending, "PENDING"},
		{Processing, "PROCESSING"},
		{Success, "SUCCESS"},
		{Partial, "PARTIAL"},
		{Failed, "FAILED"},
		{Suspended, "SUSPENDED"},
		{Dead, "DEAD"},
		{Resolved, "RESOLVED"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.s.String(); got != tt.want {
				t.Errorf("Status.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStatus_IsTerminal(t *testing.T) {
	tests := []struct {
		s    Status
		want bool
	}{
		{Pending, false},
		{Processing, false},
		{Success, true},
		{Partial, false},
		{Failed, false},
		{Suspended, false},
		{Dead, true},
		{Resolved, true},
	}
	for _, tt := range tests {
		t.Run(tt.s.String(), func(t *testing.T) {
			if got := tt.s.IsTerminal(); got != tt.want {
				t.Errorf("Status(%v).IsTerminal() = %v, want %v", tt.s, got, tt.want)
			}
		})
	}
}

func TestStatus_IsClaimable(t *testing.T) {
	tests := []struct {
		s    Status
		want bool
	}{
		{Pending, false},
		{Processing, false},
		{Success, false},
		{Partial, true},
		{Failed, true},
		{Suspended, true},
		{Dead, false},
		{Resolved, false},
	}
	for _, tt := range tests {
		t.Run(tt.s.String(), func(t *testing.T) {
			if got := tt.s.IsClaimable(); got != tt.want {
				t.Errorf("Status(%v).IsClaimable() = %v, want %v", tt.s, got, tt.want)
			}
		})
	}
}

func TestStatus_IsException(t *testing.T) {
	tests := []struct {
		s    Status
		want bool
	}{
		{Pending, false},
		{Processing, false},
		{Success, false},
		{Partial, true},
		{Failed, true},
		{Suspended, true},
		{Dead, true},
		{Resolved, true},
	}
	for _, tt := range tests {
		t.Run(tt.s.String(), func(t *testing.T) {
			if got := tt.s.IsException(); got != tt.want {
				t.Errorf("Status(%v).IsException() = %v, want %v", tt.s, got, tt.want)
			}
		})
	}
}

func TestStatus_IsError(t *testing.T) {
	tests := []struct {
		s    Status
		want bool
	}{
		{Pending, false},
		{Processing, false},
		{Success, false},
		{Partial, false},
		{Failed, true},
		{Suspended, true},
		{Dead, true},
		{Resolved, false},
	}
	for _, tt := range tests {
		t.Run(tt.s.String(), func(t *testing.T) {
			if got := tt.s.IsError(); got != tt.want {
				t.Errorf("Status(%v).IsError() = %v, want %v", tt.s, got, tt.want)
			}
		})
	}
}

func TestCanTransition(t *testing.T) {
	tests := []struct {
		name string
		from Status
		to   Status
		want bool
	}{
		// 合法转移
		{"pending->processing", Pending, Processing, true},
		{"processing->success", Processing, Success, true},
		{"processing->partial", Processing, Partial, true},
		{"processing->failed", Processing, Failed, true},
		{"processing->suspended", Processing, Suspended, true},
		{"failed->processing", Failed, Processing, true},
		{"partial->processing", Partial, Processing, true},
		{"suspended->processing", Suspended, Processing, true},
		{"failed->dead", Failed, Dead, true},
		{"partial->dead", Partial, Dead, true},
		{"suspended->dead", Suspended, Dead, true},
		{"dead->resolved", Dead, Resolved, true},

		// 非法转移
		{"pending->success", Pending, Success, false},
		{"pending->dead", Pending, Dead, false},
		{"success->processing", Success, Processing, false},
		{"success->failed", Success, Failed, false},
		{"dead->processing", Dead, Processing, false},
		{"resolved->processing", Resolved, Processing, false},
		{"resolved->success", Resolved, Success, false},
		{"partial->success", Partial, Success, false},
		{"unknown->unknown", Status("UNKNOWN"), Status("OTHER"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CanTransition(tt.from, tt.to); got != tt.want {
				t.Errorf("CanTransition(%v, %v) = %v, want %v", tt.from, tt.to, got, tt.want)
			}
		})
	}
}

func TestSyncToOrderStatus(t *testing.T) {
	tests := []struct {
		s    Status
		want string
	}{
		{Success, "SUCCESS"},
		{Failed, "FAILED"},
		{Dead, "FAILED"},
		{Suspended, "FAILED"},
		{Pending, ""},
		{Processing, ""},
		{Partial, ""},
		{Resolved, ""},
	}
	for _, tt := range tests {
		t.Run(tt.s.String(), func(t *testing.T) {
			if got := SyncToOrderStatus(tt.s); got != tt.want {
				t.Errorf("SyncToOrderStatus(%v) = %v, want %v", tt.s, got, tt.want)
			}
		})
	}
}

func TestExceptionStatuses(t *testing.T) {
	if len(ExceptionStatuses) != 5 {
		t.Errorf("ExceptionStatuses length = %d, want 5", len(ExceptionStatuses))
	}
	expected := []Status{Failed, Partial, Suspended, Dead, Resolved}
	for i, s := range ExceptionStatuses {
		if s != expected[i] {
			t.Errorf("ExceptionStatuses[%d] = %v, want %v", i, s, expected[i])
		}
	}
}