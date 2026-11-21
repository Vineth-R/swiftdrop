package httpx

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestRateLimiterSlidingWindow(t *testing.T) {
	limiter := NewRateLimiter(2, time.Minute)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	limiter.now = func() time.Time { return now }
	if _, retry := limiter.allow("a"); retry != 0 {
		t.Fatal("first request refused")
	}
	now = now.Add(30 * time.Second)
	if _, retry := limiter.allow("a"); retry != 0 {
		t.Fatal("second request refused")
	}
	if _, retry := limiter.allow("a"); retry != 30*time.Second {
		t.Fatalf("retry = %v", retry)
	}
	if _, retry := limiter.allow("b"); retry != 0 {
		t.Fatal("independent client refused")
	}
	now = now.Add(30 * time.Second)
	if _, retry := limiter.allow("a"); retry != 0 {
		t.Fatal("expired request was retained")
	}
	if _, retry := limiter.allow("a"); retry != 30*time.Second {
		t.Fatalf("window boundary bypass: %v", retry)
	}
}
func TestRateLimiterConcurrentAndBounded(t *testing.T) {
	limiter := NewRateLimiter(30, time.Minute)
	var accepted atomic.Int32
	var group sync.WaitGroup
	for range 100 {
		group.Add(1)
		go func() {
			defer group.Done()
			if _, retry := limiter.allow("same"); retry == 0 {
				accepted.Add(1)
			}
		}()
	}
	group.Wait()
	if accepted.Load() != 30 {
		t.Fatalf("accepted %d", accepted.Load())
	}
	for index := 0; index < 5000; index++ {
		limiter.allow(fmt.Sprint(index))
	}
	if len(limiter.clients) != 4096 {
		t.Fatalf("unbounded clients %d", len(limiter.clients))
	}
}

func TestRateLimiterMiddlewareKeyed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	limiter := NewRateLimiter(1, time.Minute)
	router := gin.New()
	router.GET("/keyed", limiter.MiddlewareKeyed(func(c *gin.Context) string {
		return c.GetHeader("X-Test-Key")
	}), func(c *gin.Context) { c.Status(204) })

	request := func(key string) int {
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/keyed", nil)
		if key != "" {
			req.Header.Set("X-Test-Key", key)
		}
		router.ServeHTTP(recorder, req)
		return recorder.Code
	}

	if code := request("a"); code != 204 {
		t.Fatalf("first keyed request = %d", code)
	}
	if code := request("a"); code != 429 {
		t.Fatalf("second keyed request = %d", code)
	}
	if code := request("b"); code != 204 {
		t.Fatalf("independent keyed request = %d", code)
	}
}
