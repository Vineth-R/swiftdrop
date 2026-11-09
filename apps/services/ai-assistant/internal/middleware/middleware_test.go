package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Vineth-R/swiftdrop/apps/services/ai-assistant/internal/contract"
	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func newVerifier(t *testing.T, secret string) (*auth.TokenManager, *time.Time) {
	t.Helper()
	now := time.Date(2026, 11, 8, 10, 0, 0, 0, time.UTC)
	manager, err := auth.NewTokenManager(secret, time.Minute, auth.WithClock(func() time.Time { return now }))
	if err != nil {
		t.Fatalf("NewTokenManager: %v", err)
	}
	return manager, &now
}

func authRouter(verifier auth.Verifier, cookie string) *gin.Engine {
	router := gin.New()
	router.Use(OptionalAuth(verifier, cookie))
	router.GET("/", func(c *gin.Context) {
		caller := CallerFrom(c)
		c.JSON(200, gin.H{
			"userID":        caller.UserID,
			"role":          caller.Role,
			"authenticated": caller.Authenticated,
			"token":         caller.AccessToken != "",
		})
	})
	return router
}

func request(router *gin.Engine, headers map[string]string, remote string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = remote
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	return recorder
}

func TestOptionalAuthAnonymous(t *testing.T) {
	router := authRouter(nil, "sd_access")
	recorder := request(router, nil, "203.0.113.1:1234")
	if recorder.Code != 200 {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), `"authenticated":false`) {
		t.Fatalf("body = %s", recorder.Body.String())
	}
}

func TestOptionalAuthValidBearer(t *testing.T) {
	manager, _ := newVerifier(t, strings.Repeat("s", 32))
	token, _, err := manager.Issue("user-1", auth.RoleCustomer)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	router := authRouter(manager, "sd_access")
	recorder := request(router, map[string]string{"Authorization": "Bearer " + token}, "203.0.113.1:1234")
	if recorder.Code != 200 {
		t.Fatalf("status = %d, want 200: %s", recorder.Code, recorder.Body.String())
	}
	body := recorder.Body.String()
	if !strings.Contains(body, `"userID":"user-1"`) || !strings.Contains(body, `"authenticated":true`) {
		t.Fatalf("body = %s", body)
	}
}

func TestOptionalAuthValidCookie(t *testing.T) {
	manager, _ := newVerifier(t, strings.Repeat("s", 32))
	token, _, _ := manager.Issue("user-2", auth.RoleAdmin)
	router := authRouter(manager, "sd_access")
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "sd_access", Value: token})
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	if recorder.Code != 200 || !strings.Contains(recorder.Body.String(), `"userID":"user-2"`) {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestOptionalAuthInvalidToken(t *testing.T) {
	manager, _ := newVerifier(t, strings.Repeat("s", 32))
	router := authRouter(manager, "sd_access")
	recorder := request(router, map[string]string{"Authorization": "Bearer not-a-token"}, "203.0.113.1:1234")
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", recorder.Code)
	}
}

func TestOptionalAuthExpiredToken(t *testing.T) {
	manager, now := newVerifier(t, strings.Repeat("s", 32))
	token, _, _ := manager.Issue("user-1", auth.RoleCustomer)
	*now = now.Add(2 * time.Minute)

	router := authRouter(manager, "sd_access")
	recorder := request(router, map[string]string{"Authorization": "Bearer " + token}, "203.0.113.1:1234")
	if recorder.Code != http.StatusUnauthorized || !strings.Contains(recorder.Body.String(), "TOKEN_EXPIRED") {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestOptionalAuthMalformedHeader(t *testing.T) {
	manager, _ := newVerifier(t, strings.Repeat("s", 32))
	router := authRouter(manager, "sd_access")
	recorder := request(router, map[string]string{"Authorization": "Basic abc"}, "203.0.113.1:1234")
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", recorder.Code)
	}
}

func TestOptionalAuthCredentialsWithoutVerifierFailClosed(t *testing.T) {
	router := authRouter(nil, "sd_access")
	recorder := request(router, map[string]string{"Authorization": "Bearer token"}, "203.0.113.1:1234")
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", recorder.Code)
	}
}

func TestLimiterConcurrentAccess(t *testing.T) {
	limiter := NewLimiter(10, time.Minute)
	var allowed atomic.Int32
	var wait sync.WaitGroup
	wait.Add(100)
	for i := 0; i < 100; i++ {
		go func() {
			defer wait.Done()
			remaining, retry := limiter.allow("user:concurrent")
			if remaining >= 0 && retry == 0 {
				allowed.Add(1)
			}
		}()
	}
	wait.Wait()
	if got := allowed.Load(); got != 10 {
		t.Fatalf("allowed = %d, want 10", got)
	}
}

func TestLimiterPerUserAndAnonymousIP(t *testing.T) {
	limiter := NewLimiter(2, time.Minute)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		if header := c.GetHeader("X-Test-User"); header != "" {
			c.Set(callerContextKey, contract.CallerContext{UserID: header, Authenticated: true})
		}
		c.Next()
	})
	router.Use(limiter.Middleware())
	router.GET("/", func(c *gin.Context) { c.Status(200) })

	for i := 0; i < 2; i++ {
		if rec := request(router, map[string]string{"X-Test-User": "a"}, "203.0.113.1:1"); rec.Code != 200 {
			t.Fatalf("request %d status = %d", i, rec.Code)
		}
	}
	rec := request(router, map[string]string{"X-Test-User": "a"}, "203.0.113.1:1")
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", rec.Code)
	}
	if rec.Header().Get("Retry-After") == "" || rec.Header().Get("X-RateLimit-Remaining") != "0" {
		t.Fatalf("missing rate limit headers: %v", rec.Header())
	}

	// A different authenticated user has an independent budget.
	if rec := request(router, map[string]string{"X-Test-User": "b"}, "203.0.113.1:1"); rec.Code != 200 {
		t.Fatalf("second user status = %d, want 200", rec.Code)
	}

	// Anonymous callers are isolated by IP.
	for i := 0; i < 2; i++ {
		if rec := request(router, nil, "203.0.113.2:1"); rec.Code != 200 {
			t.Fatalf("anonymous request %d status = %d", i, rec.Code)
		}
	}
	if rec := request(router, nil, "203.0.113.2:1"); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("anonymous status = %d, want 429", rec.Code)
	}
	if rec := request(router, nil, "203.0.113.3:1"); rec.Code != 200 {
		t.Fatalf("other IP status = %d, want 200", rec.Code)
	}
}
