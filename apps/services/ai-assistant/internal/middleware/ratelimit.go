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
// shared IP limiter's fail-closed behaviour when the key table is full.
type Limiter struct {
	mu      sync.Mutex
	limit   int
	window  time.Duration
	clients map[string][]time.Time
	now     func() time.Time
}

// NewLimiter returns a limiter for the given per-window budget.
func NewLimiter(limit int, window time.Duration) *Limiter {
	return &Limiter{limit: limit, window: window, clients: make(map[string][]time.Time), now: time.Now}
}

// Middleware enforces the limiter and writes Retry-After on rejection.
func (l *Limiter) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		remaining, retry := l.allow(keyFor(c))
		c.Header("X-RateLimit-Remaining", strconv.Itoa(remaining))
		if retry > 0 {
			c.Header("Retry-After", strconv.Itoa(int(math.Ceil(retry.Seconds()))))
			httpx.Error(c, http.StatusTooManyRequests, "RATE_LIMITED", "too many requests")
			return
		}
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

func (l *Limiter) allow(key string) (int, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	if l.limit < 1 || l.window <= 0 {
		return 0, l.window
	}

	requests, exists := l.clients[key]
	if !exists && len(l.clients) >= maxLimiterKeys {
		for client, times := range l.clients {
			if len(times) == 0 || !times[len(times)-1].Add(l.window).After(now) {
				delete(l.clients, client)
			}
		}
		if len(l.clients) >= maxLimiterKeys {
			return 0, l.window
		}
	}

	first := 0
	for first < len(requests) && !requests[first].Add(l.window).After(now) {
		first++
	}
	requests = append(requests[:0], requests[first:]...)

	if len(requests) >= l.limit {
		return 0, requests[0].Add(l.window).Sub(now)
	}

	requests = append(requests, now)
	l.clients[key] = requests
	return l.limit - len(requests), 0
}
