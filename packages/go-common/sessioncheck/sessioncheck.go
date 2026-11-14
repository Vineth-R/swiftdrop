// Package sessioncheck verifies that an access token still represents the
// account's current state.
//
// Identity is authoritative for a user's role, active flag and session version
// (the JWT "sv" claim). A signed access token is valid until it expires, so a
// suspension, deactivation, role change or session-version bump only takes
// effect in other services when they re-check identity. This package calls the
// existing identity internal endpoint `GET /internal/users/{userId}/session-state`
// and caches the answer for a short, bounded time.
//
// # Revocation guarantee
//
// A revoked session is rejected within CacheTTL of the revocation (30s by
// default). A cache hit is never older than CacheTTL, so the worst-case delay
// is CacheTTL plus one identity round trip. Once the cached value expires, a
// fresh authoritative answer is required; there is no stale-authorization
// fallback.
//
// # Failure policy
//
// The check fails closed. When identity is unavailable and the cached value is
// older than CacheTTL, the request is refused with ErrUnavailable (mapped to
// 503), matching the merchant service's existing behaviour. Cached state is
// never used to authorize a request beyond CacheTTL.
package sessioncheck

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
	"github.com/Vineth-R/swiftdrop/packages/go-common/requestid"
)

var (
	// ErrUnavailable means identity could not confirm the session. Callers must
	// fail closed and never treat this as "still valid".
	ErrUnavailable = errors.New("identity session state is unavailable")
	// ErrRevoked means the account is inactive, its role changed, or the token
	// carries an old session version.
	ErrRevoked = errors.New("session is no longer valid")
)

const (
	DefaultCacheTTL   = 30 * time.Second
	DefaultMaxEntries = 10000
)

// State is the authoritative identity view of one user.
type State struct {
	Active         bool
	Role           auth.Role
	SessionVersion int
}

// Guard verifies that claims still match identity. *Checker implements it;
// tests use a small permissive stand-in.
type Guard interface {
	Check(ctx context.Context, claims auth.Claims) error
}

// Fetcher retrieves authoritative session state. *Client implements it; tests
// supply fakes.
type Fetcher interface {
	Fetch(ctx context.Context, userID string) (State, error)
}

// Option customises a Checker.
type Option func(*Checker)

// WithCacheTTL sets how long a successful lookup is trusted.
func WithCacheTTL(ttl time.Duration) Option {
	return func(c *Checker) {
		if ttl > 0 {
			c.ttl = ttl
		}
	}
}

// WithMaxEntries bounds the cache. When full, the oldest entry is evicted.
func WithMaxEntries(max int) Option {
	return func(c *Checker) {
		if max > 0 {
			c.max = max
		}
	}
}

// WithClock replaces the time source for deterministic tests.
func WithClock(now func() time.Time) Option {
	return func(c *Checker) {
		if now != nil {
			c.now = now
		}
	}
}

// Checker caches identity session state and compares it with verified claims.
type Checker struct {
	fetcher Fetcher
	ttl     time.Duration
	max     int
	now     func() time.Time

	mu       sync.Mutex
	entries  map[string]entry
	inflight map[string]*call
}

type entry struct {
	state State
	at    time.Time
}

type call struct {
	done  chan struct{}
	state State
	err   error
}

// New returns a checker. A nil fetcher makes every check fail closed.
func New(fetcher Fetcher, opts ...Option) *Checker {
	checker := &Checker{
		fetcher:  fetcher,
		ttl:      DefaultCacheTTL,
		max:      DefaultMaxEntries,
		now:      time.Now,
		entries:  make(map[string]entry),
		inflight: make(map[string]*call),
	}
	for _, opt := range opts {
		opt(checker)
	}
	return checker
}

// Check reports whether claims still match the current identity state. It never
// accepts caller-supplied state; only the verified token and identity's answer
// are used.
func (c *Checker) Check(ctx context.Context, claims auth.Claims) error {
	if claims.UserID == "" {
		return ErrRevoked
	}

	now := c.now().UTC()
	c.mu.Lock()
	cached, present := c.entries[claims.UserID]
	c.mu.Unlock()
	if present && now.Sub(cached.at) <= c.ttl {
		return compare(claims, cached.state)
	}

	state, err := c.fetch(ctx, claims.UserID)
	if err != nil {
		// A failed refresh is never authorization success, even when a cached
		// value exists. The cache only authorizes requests younger than TTL.
		return ErrUnavailable
	}

	c.store(claims.UserID, state, now)
	return compare(claims, state)
}

// fetch deduplicates concurrent lookups for the same user so a burst after
// cache expiry does not multiply identity traffic.
func (c *Checker) fetch(ctx context.Context, userID string) (State, error) {
	c.mu.Lock()
	if existing, ok := c.inflight[userID]; ok {
		c.mu.Unlock()
		select {
		case <-existing.done:
			return existing.state, existing.err
		case <-ctx.Done():
			return State{}, ErrUnavailable
		}
	}
	current := &call{done: make(chan struct{})}
	c.inflight[userID] = current
	c.mu.Unlock()

	if c.fetcher == nil {
		c.finishCall(userID, current, State{}, ErrUnavailable)
		return State{}, ErrUnavailable
	}
	state, err := c.fetcher.Fetch(ctx, userID)
	c.finishCall(userID, current, state, err)
	return state, err
}

func (c *Checker) finishCall(userID string, current *call, state State, err error) {
	c.mu.Lock()
	current.state = state
	current.err = err
	delete(c.inflight, userID)
	close(current.done)
	c.mu.Unlock()
}

func (c *Checker) store(userID string, state State, at time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.entries) >= c.max {
		c.evictLocked(at)
	}
	c.entries[userID] = entry{state: state, at: at}
}

func (c *Checker) evictLocked(now time.Time) {
	for key, value := range c.entries {
		if now.Sub(value.at) > c.ttl {
			delete(c.entries, key)
		}
	}
	if len(c.entries) < c.max {
		return
	}
	var oldestKey string
	var oldest time.Time
	for key, value := range c.entries {
		if oldestKey == "" || value.at.Before(oldest) {
			oldestKey, oldest = key, value.at
		}
	}
	if oldestKey != "" {
		delete(c.entries, oldestKey)
	}
}

func compare(claims auth.Claims, state State) error {
	if !state.Active || state.Role != claims.Role || state.SessionVersion != claims.SessionVersion {
		return ErrRevoked
	}
	return nil
}

// Client is the identity internal session-state HTTP client.
type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

// NewClient builds a client. An empty token is allowed; identity then answers
// 401 and every check fails closed.
func NewClient(baseURL, token string, timeout time.Duration) (*Client, error) {
	trimmed := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, errors.New("IDENTITY_BASE_URL must be an absolute http or https URL")
	}
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	return &Client{
		baseURL: trimmed,
		token:   strings.TrimSpace(token),
		http: &http.Client{
			Timeout: timeout,
			// An internal call that redirects is a misconfiguration; following
			// it could send the internal token to another host.
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}, nil
}

type stateEnvelope struct {
	Data struct {
		Role           string `json:"role"`
		IsActive       bool   `json:"isActive"`
		SessionVersion int    `json:"sessionVersion"`
	} `json:"data"`
}

// Fetch calls identity's internal session-state endpoint.
func (c *Client) Fetch(ctx context.Context, userID string) (State, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.baseURL+"/internal/users/"+url.PathEscape(userID)+"/session-state", nil)
	if err != nil {
		return State{}, ErrUnavailable
	}
	request.Header.Set("X-Internal-Token", c.token)
	request.Header.Set(requestid.Header, outboundRequestID(ctx))

	response, err := c.http.Do(request)
	if err != nil {
		return State{}, ErrUnavailable
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return State{}, ErrUnavailable
	}
	var envelope stateEnvelope
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		return State{}, ErrUnavailable
	}
	role, ok := auth.ParseRole(envelope.Data.Role)
	if !ok {
		return State{}, ErrUnavailable
	}
	return State{
		Active:         envelope.Data.IsActive,
		Role:           role,
		SessionVersion: envelope.Data.SessionVersion,
	}, nil
}

func outboundRequestID(ctx context.Context) string {
	if id := requestid.FromContext(ctx); id != "" {
		return id
	}
	return requestid.New()
}

// Describe returns a safe one-word reason for logs without including identity
// fields or personal data.
func Describe(err error) string {
	switch {
	case errors.Is(err, ErrRevoked):
		return "revoked"
	case errors.Is(err, ErrUnavailable):
		return "unavailable"
	default:
		return "error"
	}
}
