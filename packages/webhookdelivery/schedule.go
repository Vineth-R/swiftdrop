package webhookdelivery

import "time"

// MaxAttempts is the number of delivery attempts per event and webhook. The
// database enforces the same limit (attempt_number between 1 and 6).
const MaxAttempts = 6

// retryDelays[n-1] is the wait after attempt n fails, before attempt n+1
// (API v1.1, section 10.3): 1 minute, 5 minutes, 30 minutes, 2 hours, 6 hours.
// Attempt 1 is immediate, so it has no entry of its own.
var retryDelays = [MaxAttempts - 1]time.Duration{
	1 * time.Minute,
	5 * time.Minute,
	30 * time.Minute,
	2 * time.Hour,
	6 * time.Hour,
}

// DelayAfterFailure returns how long to wait after attempt number `attempt`
// (1-based) failed before trying again. ok is false when no attempts remain:
// attempt 6 failed (or the number is out of range), so the event is dead
// lettered instead of retried.
func DelayAfterFailure(attempt int) (delay time.Duration, ok bool) {
	if attempt < 1 || attempt >= MaxAttempts {
		return 0, false
	}
	return retryDelays[attempt-1], true
}
