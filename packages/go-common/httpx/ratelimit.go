package httpx

import (
	"math"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// RateLimiter is a bounded, process-local sliding-window limiter.
// Full capacity fails closed instead of evicting active clients and allowing bypass.
type RateLimiter struct {
	mu      sync.Mutex
	limit   int
	window  time.Duration
	clients map[string][]time.Time
	now     func() time.Time
}

func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	return &RateLimiter{limit: limit, window: window, clients: make(map[string][]time.Time), now: time.Now}
}

func (l *RateLimiter) Middleware() gin.HandlerFunc {
	return l.MiddlewareKeyed(func(c *gin.Context) string { return c.ClientIP() })
}

// MiddlewareKeyed lets an authenticated route limit by trusted identity instead
// of the shared gateway address. The key function must return a non-empty value;
// an empty key falls back to the client IP so a missing claim cannot bypass the
// limiter.
func (l *RateLimiter) MiddlewareKeyed(key func(*gin.Context) string) gin.HandlerFunc {
	return func(c *gin.Context) {
		client := key(c)
		if client == "" {
			client = c.ClientIP()
		}
		remaining, retry := l.allow(client)
		c.Header("X-RateLimit-Remaining", strconv.Itoa(remaining))
		if retry > 0 {
			c.Header("Retry-After", strconv.Itoa(int(math.Ceil(retry.Seconds()))))
			Error(c, http.StatusTooManyRequests, "RATE_LIMITED", "too many requests")
			return
		}
		c.Next()
	}
}

func (l *RateLimiter) allow(key string) (int, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	if l.limit < 1 || l.window <= 0 {
		return 0, time.Minute
	}
	requests, exists := l.clients[key]
	if !exists && len(l.clients) >= 4096 {
		for client, times := range l.clients {
			if !times[len(times)-1].Add(l.window).After(now) {
				delete(l.clients, client)
			}
		}
		if len(l.clients) >= 4096 {
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
