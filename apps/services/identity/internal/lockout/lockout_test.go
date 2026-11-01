package lockout

import (
	"testing"
	"time"
)

var now = time.Date(2026, time.November, 2, 9, 30, 0, 0, time.UTC)

func timePtr(value time.Time) *time.Time { return &value }

func TestState_Locked(t *testing.T) {
	tests := []struct {
		name  string
		state State
		want  bool
	}{
		{name: "never locked", state: State{FailedCount: 3}, want: false},
		{name: "lock in the future", state: State{LockedUntil: timePtr(now.Add(time.Minute))}, want: true},
		{name: "lock ends exactly now", state: State{LockedUntil: timePtr(now)}, want: false},
		{name: "lock in the past", state: State{LockedUntil: timePtr(now.Add(-time.Minute))}, want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.state.Locked(now); got != test.want {
				t.Fatalf("Locked() = %t, want %t", got, test.want)
			}
		})
	}
}

func TestPolicy_AfterFailure(t *testing.T) {
	policy := Policy{MaxFailures: 5, Duration: 15 * time.Minute}

	t.Run("counts failures below the limit without locking", func(t *testing.T) {
		state := State{}
		for attempt := 1; attempt < policy.MaxFailures; attempt++ {
			state = policy.AfterFailure(state, now)
			if state.FailedCount != attempt || state.LockedUntil != nil {
				t.Fatalf("after attempt %d state = %+v, want count %d and no lock", attempt, state, attempt)
			}
		}
	})

	t.Run("locks on the failure that reaches the limit", func(t *testing.T) {
		state := policy.AfterFailure(State{FailedCount: 4}, now)
		if state.FailedCount != 5 {
			t.Fatalf("FailedCount = %d, want 5", state.FailedCount)
		}
		if state.LockedUntil == nil || !state.LockedUntil.Equal(now.Add(15*time.Minute)) {
			t.Fatalf("LockedUntil = %v, want %v", state.LockedUntil, now.Add(15*time.Minute))
		}
	})

	t.Run("a failure during a lock does not extend it", func(t *testing.T) {
		locked := State{FailedCount: 5, LockedUntil: timePtr(now.Add(10 * time.Minute))}
		got := policy.AfterFailure(locked, now.Add(time.Minute))
		if got.FailedCount != 5 || !got.LockedUntil.Equal(*locked.LockedUntil) {
			t.Fatalf("state = %+v, want unchanged %+v", got, locked)
		}
	})

	t.Run("an expired lock restarts counting at one", func(t *testing.T) {
		expired := State{FailedCount: 5, LockedUntil: timePtr(now.Add(-time.Second))}
		got := policy.AfterFailure(expired, now)
		if got.FailedCount != 1 || got.LockedUntil != nil {
			t.Fatalf("state = %+v, want count 1 and no lock", got)
		}
	})

	t.Run("a limit of one locks immediately", func(t *testing.T) {
		got := Policy{MaxFailures: 1, Duration: time.Minute}.AfterFailure(State{}, now)
		if got.LockedUntil == nil {
			t.Fatal("account was not locked")
		}
	})
}
