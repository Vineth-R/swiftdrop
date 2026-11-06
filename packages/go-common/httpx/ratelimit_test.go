package httpx

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
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
