// Package lockout holds the account lockout rule: after a number of
// consecutive failed logins the account is locked for a fixed period (API v1.1
// section 2.2, 423 ACCOUNT_LOCKED). The rule is pure so it can be tested
// without a database; the repository applies it inside a transaction.
package lockout

import "time"

// Policy is the lockout configuration.
type Policy struct {
	// MaxFailures is the number of consecutive failed logins that locks the account.
	MaxFailures int
	// Duration is how long the account stays locked.
	Duration time.Duration
}

// State is the persisted lockout state of one account
// (users.failed_login_count and users.locked_until).
type State struct {
	FailedCount int
	LockedUntil *time.Time
}

// Locked reports whether the account is locked at now.
func (s State) Locked(now time.Time) bool {
	return s.LockedUntil != nil && now.Before(*s.LockedUntil)
}

// AfterFailure returns the state after one more failed login at now.
//
// A lock that has already expired is forgotten, so counting restarts at one.
// Failures while the account is locked do not extend the lock.
func (p Policy) AfterFailure(state State, now time.Time) State {
	if state.LockedUntil != nil && !state.Locked(now) {
		state = State{}
	}
	if state.Locked(now) {
		return state
	}

	next := State{FailedCount: state.FailedCount + 1}
	if next.FailedCount >= p.MaxFailures {
		until := now.Add(p.Duration)
		next.LockedUntil = &until
	}
	return next
}
