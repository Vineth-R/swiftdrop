package servicehttp

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestRateLimiter_allow(t *testing.T) {
	now := time.Date(2026, time.November, 2, 9, 30, 0, 0, time.UTC)
	limiter := newRateLimiter(3, time.Minute, func() time.Time { return now })

	t.Run("allows up to the limit and reports the remainder", func(t *testing.T) {
		for want := 2; want >= 0; want-- {
			allowed, remaining, _ := limiter.allow("a")
			if !allowed || remaining != want {
				t.Fatalf("allowed=%t remaining=%d, want allowed with %d left", allowed, remaining, want)
			}
		}
	})

	t.Run("refuses the next request and says when to retry", func(t *testing.T) {
		now = now.Add(20 * time.Second)
		allowed, remaining, resetIn := limiter.allow("a")
		if allowed || remaining != 0 {
			t.Fatalf("allowed=%t remaining=%d, want refused", allowed, remaining)
		}
		if resetIn != 40*time.Second {
			t.Fatalf("resetIn = %s, want 40s", resetIn)
		}
	})

	t.Run("keys are independent", func(t *testing.T) {
		if allowed, _, _ := limiter.allow("b"); !allowed {
			t.Fatal("another key was refused")
		}
	})

	t.Run("the window resets", func(t *testing.T) {
		now = now.Add(time.Minute)
		if allowed, _, _ := limiter.allow("a"); !allowed {
			t.Fatal("the key was still refused after the window")
		}
	})
}

func TestRateLimiter_sweepsExpiredWindows(t *testing.T) {
	now := time.Date(2026, time.November, 2, 9, 30, 0, 0, time.UTC)
	limiter := newRateLimiter(1, time.Minute, func() time.Time { return now })

	for i := range sweepThreshold {
		limiter.allow("old-" + strconv.Itoa(i))
	}
	now = now.Add(2 * time.Minute)
	limiter.allow("fresh")

	if got := len(limiter.entries); got != 1 {
		t.Fatalf("tracked keys = %d, want only the fresh one after the sweep", got)
	}
}

func TestLimitByClient(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.TrustedPlatform = "X-Real-IP"
	router.Use(limitByClient(newRateLimiter(2, time.Minute, nil)))
	router.GET("/x", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	call := func(ip string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodGet, "/x", nil)
		request.Header.Set("X-Real-IP", ip)
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		return recorder
	}

	if first := call("203.0.113.1"); first.Code != http.StatusNoContent || first.Header().Get("X-RateLimit-Remaining") != "1" {
		t.Fatalf("first call: status %d remaining %q", first.Code, first.Header().Get("X-RateLimit-Remaining"))
	}
	call("203.0.113.1")

	blocked := call("203.0.113.1")
	if blocked.Code != http.StatusTooManyRequests {
		t.Fatalf("third call status = %d, want %d", blocked.Code, http.StatusTooManyRequests)
	}
	retryAfter, err := strconv.Atoi(blocked.Header().Get("Retry-After"))
	if err != nil || retryAfter < 1 || retryAfter > 61 {
		t.Fatalf("Retry-After = %q", blocked.Header().Get("Retry-After"))
	}

	if other := call("203.0.113.2"); other.Code != http.StatusNoContent {
		t.Fatalf("another client was limited: status %d", other.Code)
	}
}
