package webhookdelivery

import (
	"testing"
	"time"
)

// API v1.1, section 10.3: attempt 1 now, then 1 min, 5 min, 30 min, 2 h, 6 h.
func TestDelayAfterFailure(t *testing.T) {
	tests := []struct {
		attempt int
		delay   time.Duration
		ok      bool
	}{
		{1, 1 * time.Minute, true},
		{2, 5 * time.Minute, true},
		{3, 30 * time.Minute, true},
		{4, 2 * time.Hour, true},
		{5, 6 * time.Hour, true},
		{6, 0, false}, // last attempt failed: dead letter
		{7, 0, false},
		{0, 0, false},
		{-1, 0, false},
	}
	for _, tc := range tests {
		delay, ok := DelayAfterFailure(tc.attempt)
		if delay != tc.delay || ok != tc.ok {
			t.Errorf("DelayAfterFailure(%d) = %v, %v; want %v, %v", tc.attempt, delay, ok, tc.delay, tc.ok)
		}
	}
}

func TestMaxAttemptsMatchesDatabaseLimit(t *testing.T) {
	if MaxAttempts != 6 {
		t.Fatalf("MaxAttempts = %d; the database CHECK allows attempt_number 1 to 6", MaxAttempts)
	}
}
