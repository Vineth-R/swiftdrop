package sessioncheck

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
)

type fakeFetcher struct {
	mu     sync.Mutex
	state  State
	err    error
	calls  int
	delay  time.Duration
	before func()
}

func (f *fakeFetcher) Fetch(context.Context, string) (State, error) {
	if f.before != nil {
		f.before()
	}
	if f.delay > 0 {
		time.Sleep(f.delay)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	return f.state, f.err
}

func (f *fakeFetcher) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func claims(version int) auth.Claims {
	return auth.Claims{UserID: "user-1", Role: auth.RoleCustomer, SessionVersion: version}
}

func TestCheckAcceptsCurrentSession(t *testing.T) {
	fetcher := &fakeFetcher{state: State{Active: true, Role: auth.RoleCustomer, SessionVersion: 3}}
	checker := New(fetcher)

	if err := checker.Check(context.Background(), claims(3)); err != nil {
		t.Fatalf("Check() error = %v, want nil", err)
	}
}

func TestCheckRejectsRevokedSession(t *testing.T) {
	tests := []struct {
		name  string
		state State
	}{
		{"inactive", State{Active: false, Role: auth.RoleCustomer, SessionVersion: 3}},
		{"stale version", State{Active: true, Role: auth.RoleCustomer, SessionVersion: 4}},
		{"role changed", State{Active: true, Role: auth.RoleAdmin, SessionVersion: 3}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			checker := New(&fakeFetcher{state: test.state})
			if err := checker.Check(context.Background(), claims(3)); !errors.Is(err, ErrRevoked) {
				t.Fatalf("Check() error = %v, want ErrRevoked", err)
			}
		})
	}
}

func TestCheckFailsClosedWhenIdentityUnavailable(t *testing.T) {
	fetcher := &fakeFetcher{err: errors.New("boom")}
	checker := New(fetcher)
	if err := checker.Check(context.Background(), claims(1)); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Check() error = %v, want ErrUnavailable", err)
	}
}

func TestCheckCachesWithinTTL(t *testing.T) {
	now := time.Unix(1000, 0)
	fetcher := &fakeFetcher{state: State{Active: true, Role: auth.RoleCustomer, SessionVersion: 1}}
	checker := New(fetcher, WithClock(func() time.Time { return now }))

	for i := 0; i < 3; i++ {
		if err := checker.Check(context.Background(), claims(1)); err != nil {
			t.Fatalf("Check() error = %v", err)
		}
	}
	if got := fetcher.count(); got != 1 {
		t.Fatalf("identity calls = %d, want 1", got)
	}

	now = now.Add(DefaultCacheTTL + time.Second)
	if err := checker.Check(context.Background(), claims(1)); err != nil {
		t.Fatalf("Check() after TTL error = %v", err)
	}
	if got := fetcher.count(); got != 2 {
		t.Fatalf("identity calls after TTL = %d, want 2", got)
	}
}

func TestCheckFailsClosedWhenCachedStateExpiresAndIdentityIsDown(t *testing.T) {
	now := time.Unix(1000, 0)
	fetcher := &fakeFetcher{state: State{Active: true, Role: auth.RoleCustomer, SessionVersion: 1}}
	checker := New(fetcher, WithClock(func() time.Time { return now }))
	if err := checker.Check(context.Background(), claims(1)); err != nil {
		t.Fatalf("prime cache: %v", err)
	}

	fetcher.err = errors.New("identity down")
	now = now.Add(DefaultCacheTTL + time.Second)
	if err := checker.Check(context.Background(), claims(1)); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Check() after TTL with identity down = %v, want ErrUnavailable", err)
	}
}

func TestCheckNeverUsesExpiredCacheToAuthorize(t *testing.T) {
	now := time.Unix(1000, 0)
	fetcher := &fakeFetcher{state: State{Active: true, Role: auth.RoleCustomer, SessionVersion: 1}}
	checker := New(fetcher, WithClock(func() time.Time { return now }))
	if err := checker.Check(context.Background(), claims(1)); err != nil {
		t.Fatalf("prime cache: %v", err)
	}

	// Identity now reports a newer version. An expired cache entry must cause a
	// refresh, and the refreshed state must reject the old claims.
	fetcher.state = State{Active: true, Role: auth.RoleCustomer, SessionVersion: 2}
	now = now.Add(DefaultCacheTTL + time.Second)
	if err := checker.Check(context.Background(), claims(1)); !errors.Is(err, ErrRevoked) {
		t.Fatalf("Check() after TTL = %v, want ErrRevoked", err)
	}
}

func TestCheckDeduplicatesConcurrentLookups(t *testing.T) {
	fetcher := &fakeFetcher{
		state: State{Active: true, Role: auth.RoleCustomer, SessionVersion: 1},
		delay: 20 * time.Millisecond,
	}
	checker := New(fetcher)

	const requests = 32
	var wg sync.WaitGroup
	var failures atomic.Int32
	for i := 0; i < requests; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := checker.Check(context.Background(), claims(1)); err != nil {
				failures.Add(1)
			}
		}()
	}
	wg.Wait()

	if failures.Load() != 0 {
		t.Fatalf("concurrent checks failed: %d", failures.Load())
	}
	if got := fetcher.count(); got != 1 {
		t.Fatalf("identity calls = %d, want 1 (single-flight)", got)
	}
}

func TestCheckCacheIsBounded(t *testing.T) {
	fetcher := &fakeFetcher{state: State{Active: true, Role: auth.RoleCustomer, SessionVersion: 1}}
	checker := New(fetcher, WithMaxEntries(2))
	for i := 0; i < 10; i++ {
		user := claims(1)
		user.UserID = "user-" + string(rune('a'+i))
		if err := checker.Check(context.Background(), user); err != nil {
			t.Fatalf("Check() error = %v", err)
		}
	}
	checker.mu.Lock()
	size := len(checker.entries)
	checker.mu.Unlock()
	if size > 2 {
		t.Fatalf("cache size = %d, want at most 2", size)
	}
}

func TestNilFetcherFailsClosed(t *testing.T) {
	checker := New(nil)
	if err := checker.Check(context.Background(), claims(1)); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Check() error = %v, want ErrUnavailable", err)
	}
}
