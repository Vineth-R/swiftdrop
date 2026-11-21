package servicehttp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Vineth-R/swiftdrop/apps/services/identity/internal/service"
	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
)

// testOAuthState is a syntactically valid 43-character state, so the handler
// can derive a browser-binding cookie name from it.
const testOAuthState = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func testBindingCookieName() string {
	name, _ := service.OAuthBindingCookieName(testOAuthState)
	return name
}

type fakeGoogleService struct {
	start      service.GoogleLoginStart
	result     service.GoogleLoginResult
	err        error
	startCalls int

	gotRedirectTo string
	gotCallback   service.GoogleCallbackInput
	callbacks     int
}

func (f *fakeGoogleService) StartGoogleLogin(_ context.Context, redirectTo string) (service.GoogleLoginStart, error) {
	f.startCalls++
	f.gotRedirectTo = redirectTo
	return f.start, f.err
}

func (f *fakeGoogleService) CompleteGoogleLogin(_ context.Context, input service.GoogleCallbackInput) (service.GoogleLoginResult, error) {
	f.callbacks++
	f.gotCallback = input
	return f.result, f.err
}

type googleHarness struct {
	service *fakeGoogleService
	handler http.Handler
	logs    *bytes.Buffer
}

func newGoogleHarness(t *testing.T) *googleHarness {
	return newGoogleHarnessWithSecure(t, false)
}

func newGoogleHarnessWithSecure(t *testing.T, secure bool) *googleHarness {
	t.Helper()
	logs := &bytes.Buffer{}
	fake := &fakeGoogleService{
		start: service.GoogleLoginStart{
			AuthURL:           "https://accounts.google.com/o/oauth2/v2/auth?state=" + testOAuthState,
			BindingCookieName: testBindingCookieName(),
			BindingValue:      "binding-value",
			BindingMaxAge:     420,
		},
		result: service.GoogleLoginResult{
			Session: service.Session{
				Account:          service.Account{ID: "user-1", Email: "nimal@example.com", Role: auth.RoleCustomer},
				AccessToken:      "access-token-value",
				AccessExpiresAt:  time.Now().Add(15 * time.Minute),
				ExpiresIn:        900,
				RefreshToken:     "rt_refresh-token-value",
				RefreshExpiresAt: time.Now().Add(7 * 24 * time.Hour),
			},
		},
	}
	handler := New(slog.New(slog.NewTextHandler(logs, nil)), WithGoogle(GoogleRoutes{
		Service: fake,
		Cookies: CookieConfig{
			AccessName: "sd_access", RefreshName: "sd_refresh",
			Secure: secure, SameSite: http.SameSiteLaxMode,
		},
	}))
	return &googleHarness{service: fake, handler: handler, logs: logs}
}

func (h *googleHarness) get(target string, headers ...string) *httptest.ResponseRecorder {
	return h.getWithCookies(target, nil, headers...)
}

func (h *googleHarness) getWithCookies(target string, cookies []*http.Cookie, headers ...string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, target, nil)
	request.Header.Set("User-Agent", "test-browser")
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	for i := 0; i+1 < len(headers); i += 2 {
		request.Header.Set(headers[i], headers[i+1])
	}
	recorder := httptest.NewRecorder()
	h.handler.ServeHTTP(recorder, request)
	return recorder
}

func TestGoogleLogin_redirectsToGoogle(t *testing.T) {
	h := newGoogleHarness(t)

	response := h.get("/auth/google/login?redirectTo=" + url.QueryEscape("/parcels?tab=1"))

	if response.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302", response.Code)
	}
	if location := response.Header().Get("Location"); location != h.service.start.AuthURL {
		t.Fatalf("Location = %q, want the Google URL", location)
	}
	if h.service.gotRedirectTo != "/parcels?tab=1" {
		t.Fatalf("service got redirectTo %q", h.service.gotRedirectTo)
	}
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("login redirect must not be cached")
	}

	cookies := map[string]*http.Cookie{}
	for _, cookie := range response.Result().Cookies() {
		cookies[cookie.Name] = cookie
	}
	binding := cookies[testBindingCookieName()]
	if binding == nil {
		t.Fatalf("cookies = %v, want the browser-binding cookie", cookies)
	}
	if binding.Value != "binding-value" {
		t.Fatalf("binding cookie value = %q", binding.Value)
	}
	if !binding.HttpOnly || binding.SameSite != http.SameSiteLaxMode {
		t.Fatalf("binding cookie flags: HttpOnly=%t SameSite=%v", binding.HttpOnly, binding.SameSite)
	}
	if binding.Path != oauthBindingCookiePath || binding.MaxAge != h.service.start.BindingMaxAge {
		t.Fatalf("binding cookie path/MaxAge = %q/%d", binding.Path, binding.MaxAge)
	}
	if binding.Secure {
		t.Fatal("development configuration must not force Secure on the binding cookie")
	}
}

func TestGoogleLogin_bindingCookieIsSecureWhenConfigured(t *testing.T) {
	h := newGoogleHarnessWithSecure(t, true)

	response := h.get("/auth/google/login")
	for _, cookie := range response.Result().Cookies() {
		if cookie.Name == testBindingCookieName() && !cookie.Secure {
			t.Fatal("production-style configuration must set Secure on the binding cookie")
		}
	}
}

func TestGoogleLogin_withoutRedirectTo(t *testing.T) {
	h := newGoogleHarness(t)

	if response := h.get("/auth/google/login"); response.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302", response.Code)
	}
	if h.service.gotRedirectTo != "" {
		t.Fatalf("service got redirectTo %q, want empty", h.service.gotRedirectTo)
	}
}

func TestGoogleLogin_unsafeRedirectIsValidationError(t *testing.T) {
	h := newGoogleHarness(t)
	h.service.err = &service.ValidationError{Issues: []service.Issue{{Field: "redirectTo", Issue: "must be a relative path"}}}

	response := h.get("/auth/google/login?redirectTo=https://evil.example")

	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", response.Code)
	}
	if code := errorCode(t, response); code != "VALIDATION_ERROR" {
		t.Fatalf("code = %q, want VALIDATION_ERROR", code)
	}
	if response.Header().Get("Location") != "" {
		t.Fatal("an invalid request must not redirect")
	}
}

func TestGoogleLogin_unexpectedFailureIsGeneric500(t *testing.T) {
	h := newGoogleHarness(t)
	h.service.err = errors.New("database password=hunter2 refused")

	response := h.get("/auth/google/login")

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", response.Code)
	}
	if strings.Contains(response.Body.String(), "hunter2") {
		t.Fatal("internal error text leaked to the client")
	}
}

func TestGoogleCallback_success(t *testing.T) {
	h := newGoogleHarness(t)
	h.service.result.RedirectTo = "/parcels"

	response := h.getWithCookies(
		"/auth/google/callback?code=the-code&state="+testOAuthState,
		[]*http.Cookie{{Name: testBindingCookieName(), Value: "binding-value"}},
		"Accept", "text/html,application/xhtml+xml",
	)

	if response.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302", response.Code)
	}
	if location := response.Header().Get("Location"); location != "/parcels" {
		t.Fatalf("Location = %q, want /parcels", location)
	}
	if got := h.service.gotCallback; got.Code != "the-code" || got.State != testOAuthState ||
		got.BrowserBinding != "binding-value" || got.UserAgent != "test-browser" {
		t.Fatalf("service got %+v", got)
	}

	cookies := map[string]*http.Cookie{}
	for _, cookie := range response.Result().Cookies() {
		cookies[cookie.Name] = cookie
	}
	access, refresh := cookies["sd_access"], cookies["sd_refresh"]
	if access == nil || refresh == nil {
		t.Fatalf("cookies = %v, want sd_access and sd_refresh", cookies)
	}
	if !access.HttpOnly || !refresh.HttpOnly {
		t.Fatal("session cookies must be HttpOnly")
	}
	if access.Path != "/" || refresh.Path != refreshCookiePath {
		t.Fatalf("cookie paths = %q and %q", access.Path, refresh.Path)
	}
	if access.Value != "access-token-value" || refresh.Value != "rt_refresh-token-value" {
		t.Fatal("cookies do not carry the issued tokens")
	}
	if strings.Contains(response.Body.String(), "access-token-value") {
		t.Fatal("tokens must not appear in the redirect body")
	}
	if binding := cookies[testBindingCookieName()]; binding == nil || binding.MaxAge >= 0 {
		t.Fatalf("binding cookie = %+v, want it cleared after the callback", binding)
	}
}

func TestGoogleCallback_missingBindingIsPassedAsEmpty(t *testing.T) {
	h := newGoogleHarness(t)

	h.get("/auth/google/callback?code=c&state=" + testOAuthState)

	if h.service.gotCallback.BrowserBinding != "" {
		t.Fatalf("BrowserBinding = %q, want empty when no cookie is present", h.service.gotCallback.BrowserBinding)
	}
}

func TestGoogleCallback_defaultLanding(t *testing.T) {
	h := newGoogleHarness(t)

	response := h.getWithCookies(
		"/auth/google/callback?code=c&state="+testOAuthState,
		[]*http.Cookie{{Name: testBindingCookieName(), Value: "binding-value"}},
	)

	if location := response.Header().Get("Location"); location != googleLandingPath {
		t.Fatalf("Location = %q, want %q", location, googleLandingPath)
	}
}

func TestGoogleCallback_neverRedirectsOffSite(t *testing.T) {
	for _, bad := range []string{"https://evil.example", "//evil.example", `/\evil.example`, "evil"} {
		h := newGoogleHarness(t)
		h.service.result.RedirectTo = bad

		response := h.getWithCookies(
			"/auth/google/callback?code=c&state="+testOAuthState,
			[]*http.Cookie{{Name: testBindingCookieName(), Value: "binding-value"}},
		)

		if location := response.Header().Get("Location"); location != googleLandingPath {
			t.Fatalf("stored redirect %q produced Location %q, want %q", bad, location, googleLandingPath)
		}
	}
}

func TestGoogleCallback_browserFailuresRedirectToLogin(t *testing.T) {
	failures := map[string]error{
		"bad state":        service.ErrOAuthStateInvalid,
		"provider error":   service.ErrOAuthProvider,
		"email unverified": service.ErrGoogleEmailNotVerified,
		"disabled account": service.ErrAccountDisabled,
		"internal error":   errors.New("database unavailable"),
	}
	for name, failure := range failures {
		t.Run(name, func(t *testing.T) {
			h := newGoogleHarness(t)
			h.service.err = failure

			response := h.getWithCookies(
				"/auth/google/callback?code=c&state="+testOAuthState,
				[]*http.Cookie{{Name: testBindingCookieName(), Value: "binding-value"}},
				"Accept", "text/html",
			)

			if response.Code != http.StatusFound {
				t.Fatalf("status = %d, want 302", response.Code)
			}
			if location := response.Header().Get("Location"); location != googleFailurePath {
				t.Fatalf("Location = %q, want %q", location, googleFailurePath)
			}
			cookies := map[string]*http.Cookie{}
			for _, cookie := range response.Result().Cookies() {
				cookies[cookie.Name] = cookie
			}
			if cookies["sd_access"] != nil || cookies["sd_refresh"] != nil {
				t.Fatal("a failed login must not set session cookies")
			}
			if binding := cookies[testBindingCookieName()]; binding == nil || binding.MaxAge >= 0 {
				t.Fatalf("binding cookie = %+v, want it cleared", binding)
			}
		})
	}
}

func TestGoogleCallback_noAcceptHeaderIsTreatedAsBrowser(t *testing.T) {
	h := newGoogleHarness(t)
	h.service.err = service.ErrOAuthStateInvalid

	if response := h.get("/auth/google/callback?code=c&state=s"); response.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302", response.Code)
	}
}

func TestGoogleCallback_jsonErrorsWhenCalledDirectly(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		status   int
		wantCode string
	}{
		{name: "bad state", err: service.ErrOAuthStateInvalid, status: http.StatusBadRequest, wantCode: "OAUTH_STATE_INVALID"},
		{name: "provider error", err: service.ErrOAuthProvider, status: http.StatusBadGateway, wantCode: "OAUTH_PROVIDER_ERROR"},
		{name: "wrapped provider error", err: errors.Join(errors.New("detail"), service.ErrOAuthProvider), status: http.StatusBadGateway, wantCode: "OAUTH_PROVIDER_ERROR"},
		{name: "disabled account", err: service.ErrAccountDisabled, status: http.StatusForbidden, wantCode: "ACCOUNT_DISABLED"},
		{name: "unverified email", err: service.ErrGoogleEmailNotVerified, status: http.StatusForbidden, wantCode: "FORBIDDEN"},
		{name: "internal error", err: errors.New("boom"), status: http.StatusInternalServerError, wantCode: "INTERNAL_ERROR"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			h := newGoogleHarness(t)
			h.service.err = test.err

			response := h.get("/auth/google/callback?code=c&state="+testOAuthState, "Accept", "application/json")

			if response.Code != test.status {
				t.Fatalf("status = %d, want %d", response.Code, test.status)
			}
			if code := errorCode(t, response); code != test.wantCode {
				t.Fatalf("code = %q, want %q", code, test.wantCode)
			}
		})
	}
}

func TestGoogleCallback_googleRefusalSendsNoCode(t *testing.T) {
	h := newGoogleHarness(t)
	h.service.err = service.ErrOAuthProvider

	response := h.getWithCookies(
		"/auth/google/callback?error=access_denied&state="+testOAuthState+"&code=ignored",
		[]*http.Cookie{{Name: testBindingCookieName(), Value: "binding-value"}},
	)

	if h.service.gotCallback.Code != "" || h.service.gotCallback.State != testOAuthState {
		t.Fatalf("service got %+v, want the state but no code", h.service.gotCallback)
	}
	if location := response.Header().Get("Location"); location != googleFailurePath {
		t.Fatalf("Location = %q, want %q", location, googleFailurePath)
	}
}

func TestGoogleCallback_logsNeverContainCredentials(t *testing.T) {
	h := newGoogleHarness(t)
	h.service.err = service.ErrOAuthProvider

	h.getWithCookies(
		"/auth/google/callback?code=SECRET-CODE&state="+testOAuthState,
		[]*http.Cookie{{Name: testBindingCookieName(), Value: "SECRET-BINDING"}},
		"Accept", "text/html",
	)

	logs := h.logs.String()
	if !strings.Contains(logs, "google sign-in failed") {
		t.Fatalf("failure was not logged: %q", logs)
	}
	for _, secret := range []string{"SECRET-CODE", testOAuthState, "SECRET-BINDING"} {
		if strings.Contains(logs, secret) {
			t.Fatalf("sensitive value %q was logged: %q", secret, logs)
		}
	}
}

func TestGoogleRoutes_areRateLimited(t *testing.T) {
	h := newGoogleHarness(t)

	for i := 0; i < authAttemptsPerMinute; i++ {
		if response := h.get("/auth/google/login"); response.Code != http.StatusFound {
			t.Fatalf("request %d status = %d, want 302", i+1, response.Code)
		}
	}
	response := h.get("/auth/google/login")
	if response.Code != http.StatusTooManyRequests || errorCode(t, response) != "RATE_LIMITED" {
		t.Fatalf("status = %d, want 429 RATE_LIMITED", response.Code)
	}
	if response.Header().Get("Retry-After") == "" {
		t.Fatal("429 must carry Retry-After")
	}
}

func TestRouter_withoutGoogleHasNoGoogleRoutes(t *testing.T) {
	handler := New(slog.New(slog.DiscardHandler))

	for _, path := range []string{"/auth/google/login", "/auth/google/callback"} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		if recorder.Code != http.StatusNotFound {
			t.Fatalf("%s status = %d, want 404 when Google is not configured", path, recorder.Code)
		}
	}
}

func errorCode(t *testing.T, response *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not an error envelope: %v (%s)", err, response.Body.String())
	}
	return body.Error.Code
}
