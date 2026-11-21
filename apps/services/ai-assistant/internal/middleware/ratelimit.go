package middleware

import (
	"math"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Vineth-R/swiftdrop/packages/go-common/httpx"
)

const maxLimiterKeys = 4096

// Limiter is a bounded, process-local sliding-window limiter keyed by
// authenticated user ID or, for anonymous callers, client IP. It mirrors the
// shared IP limiter's fail-closed behaviour when the key table is full. It also
// bounds how many turns one caller may have in flight at once.
type Limiter struct {
	mu            sync.Mutex
	limit         int
	window        time.Duration
	maxConcurrent int
	clients       map[string][]time.Time
	inflight      map[string]int
	now           func() time.Time
}

// Option customises a Limiter.
type Option func(*Limiter)

// WithMaxConcurrent bounds simultaneous in-flight requests per caller. A
// non-positive value leaves concurrency unlimited.
func WithMaxConcurrent(limit int) Option {
	return func(l *Limiter) {
		l.maxConcurrent = limit
	}
}

// NewLimiter returns a limiter for the given per-window budget.
func NewLimiter(limit int, window time.Duration, opts ...Option) *Limiter {
	limiter := &Limiter{
		limit:    limit,
		window:   window,
		clients:  make(map[string][]time.Time),
		inflight: make(map[string]int),
		now:      time.Now,
	}
	for _, opt := range opts {
		opt(limiter)
	}
	return limiter
}

// Middleware enforces the limiter and writes Retry-After on rejection.
func (l *Limiter) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		key := keyFor(c)
		remaining, retry, admitted := l.acquire(key)
		c.Header("X-RateLimit-Remaining", strconv.Itoa(remaining))
		if !admitted {
			seconds := int(math.Ceil(retry.Seconds()))
			if seconds < 1 {
				seconds = 1
			}
			c.Header("Retry-After", strconv.Itoa(seconds))
			httpx.Error(c, http.StatusTooManyRequests, "RATE_LIMITED", "too many requests")
			return
		}
		defer l.release(key)
		c.Next()
	}
}

func keyFor(c *gin.Context) string {
	caller := CallerFrom(c)
	if caller.Authenticated && caller.UserID != "" {
		return "user:" + caller.UserID
	}
	return "ip:" + c.ClientIP()
}

// acquire applies the rolling-window budget and the per-key in-flight cap. The
// retry duration is the wait before the request can be retried.
func (l *Limiter) acquire(key string) (remaining int, retry time.Duration, admitted bool) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	if l.limit < 1 || l.window <= 0 {
		return 0, l.window, false
	}

	requests, exists := l.clients[key]
	if !exists && len(l.clients) >= maxLimiterKeys {
		for client, times := range l.clients {
			if len(times) == 0 || !times[len(times)-1].Add(l.window).After(now) {
				delete(l.clients, client)
				delete(l.inflight, client)
			}
		}
		if len(l.clients) >= maxLimiterKeys {
			return 0, l.window, false
		}
	}

	first := 0
	for first < len(requests) && !requests[first].Add(l.window).After(now) {
		first++
	}
	requests = append(requests[:0], requests[first:]...)

	if len(requests) >= l.limit {
		return 0, requests[0].Add(l.window).Sub(now), false
	}

	if l.maxConcurrent > 0 && l.inflight[key] >= l.maxConcurrent {
		return 0, time.Second, false
	}

	requests = append(requests, now)
	l.clients[key] = requests
	if l.maxConcurrent > 0 {
		l.inflight[key]++
	}
	return l.limit - len(requests), 0, true
}

func (l *Limiter) release(key string) {
	if l.maxConcurrent <= 0 {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.inflight[key] <= 1 {
		delete(l.inflight, key)
		return
	}
	l.inflight[key]--
}
