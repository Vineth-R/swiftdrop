package servicehttp

import (
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Vineth-R/swiftdrop/packages/go-common/httpx"
)

const (
	// authAttemptsPerMinute is the limit for /auth/login and /auth/register per
	// client address (API v1.1 section 1.7).
	authAttemptsPerMinute = 10

	// sweepThreshold bounds memory: when this many addresses are tracked, expired
	// windows are dropped.
	sweepThreshold = 10000
)

// rateLimiter is a fixed-window counter per key. It is in memory, so the limit
// applies per identity instance, which is enough for this deployment.
type rateLimiter struct {
	limit  int
	window time.Duration
	now    func() time.Time

	mu      sync.Mutex
	entries map[string]*windowCount
}

type windowCount struct {
	start time.Time
	count int
}

func newRateLimiter(limit int, window time.Duration, now func() time.Time) *rateLimiter {
	if now == nil {
		now = time.Now
	}
	return &rateLimiter{limit: limit, window: window, now: now, entries: map[string]*windowCount{}}
}

// allow counts one request for key. It reports whether the request may
// proceed, how many requests remain in the window, and how long until the
// window resets.
func (l *rateLimiter) allow(key string) (allowed bool, remaining int, resetIn time.Duration) {
	now := l.now()

	l.mu.Lock()
	defer l.mu.Unlock()

	if len(l.entries) >= sweepThreshold {
		for entryKey, entry := range l.entries {
			if now.Sub(entry.start) >= l.window {
				delete(l.entries, entryKey)
			}
		}
	}

	entry, ok := l.entries[key]
	if !ok || now.Sub(entry.start) >= l.window {
		entry = &windowCount{start: now}
		l.entries[key] = entry
	}

	resetIn = entry.start.Add(l.window).Sub(now)
	if entry.count >= l.limit {
		return false, 0, resetIn
	}
	entry.count++
	return true, l.limit - entry.count, resetIn
}

// limitByClient rejects requests over the limit with 429 RATE_LIMITED and a
// Retry-After header, and reports the remaining allowance in
// X-RateLimit-Remaining.
func limitByClient(limiter *rateLimiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		allowed, remaining, resetIn := limiter.allow(c.ClientIP())
		c.Header("X-RateLimit-Remaining", strconv.Itoa(remaining))
		if !allowed {
			retryAfter := int(resetIn/time.Second) + 1
			c.Header("Retry-After", strconv.Itoa(retryAfter))
			httpx.Error(c, http.StatusTooManyRequests, "RATE_LIMITED", "too many requests, try again later")
			return
		}
		c.Next()
	}
}
