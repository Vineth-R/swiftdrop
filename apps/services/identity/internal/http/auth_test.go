package servicehttp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/Vineth-R/swiftdrop/apps/services/identity/internal/service"
	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
)

const handlerTestSecret = "handler-test-secret-that-is-long-enough-0123456789"

// fakeService records what the handlers pass in and returns canned results.
type fakeService struct {
	account service.Account
	session service.Session
	err     error

	registered service.RegisterInput
	loggedIn   service.LoginInput
	refreshed  service.RefreshInput
	logoutUser string
	logoutTok  string
	calls      int
}

func (f *fakeService) Register(_ context.Context, input service.RegisterInput) (service.Account, error) {
	f.calls++
	f.registered = input
	return f.account, f.err
}

func (f *fakeService) Login(_ context.Context, input service.LoginInput) (service.Session, error) {
	f.calls++
	f.loggedIn = input
	return f.session, f.err
}

func (f *fakeService) Refresh(_ context.Context, input service.RefreshInput) (service.Session, error) {
	f.calls++
	f.refreshed = input
	return f.session, f.err
}

func (f *fakeService) VerifyEmail(_ context.Context, token string) error {
	f.calls++
	return f.err
}

func (f *fakeService) ResendVerification(_ context.Context, email string, _ netip.Addr) error {
	f.calls++
	return f.err
}

func (f *fakeService) Logout(_ context.Context, userID, refreshToken string) error {
	f.calls++
	f.logoutUser, f.logoutTok = userID, refreshToken
	return f.err
}

type harness struct {
	t       *testing.T
	service *fakeService
	tokens  *auth.TokenManager
	handler http.Handler
	logs    *bytes.Buffer
}

func newHarness(t *testing.T) *harness {
	t.Helper()

	tokens, err := auth.NewTokenManager(handlerTestSecret, 15*time.Minute)
	if err != nil {
		t.Fatalf("NewTokenManager returned error: %v", err)
	}
	logs := &bytes.Buffer{}
	fake := &fakeService{
		account: service.Account{
			ID: "user-1", Email: "nimal@example.com", FullName: "Nimal Perera",
			Phone: "+94771234567", Role: auth.RoleCustomer,
			CreatedAt: time.Date(2026, time.November, 2, 9, 30, 0, 0, time.UTC),
		},
		session: service.Session{
			Account:          service.Account{ID: "user-1", Email: "nimal@example.com", FullName: "Nimal Perera", Role: auth.RoleCustomer},
			AccessToken:      "access-token-value",
			AccessExpiresAt:  time.Now().Add(15 * time.Minute),
			ExpiresIn:        900,
			RefreshToken:     "rt_refresh-token-value",
			RefreshExpiresAt: time.Now().Add(7 * 24 * time.Hour),
		},
	}

	router := New(slog.New(slog.NewTextHandler(logs, nil)), WithAuth(AuthRoutes{
		Service:  fake,
		Verifier: tokens,
		Cookies:  testCookies,
	}))
	return &harness{t: t, service: fake, tokens: tokens, handler: router, logs: logs}
}

func (h *harness) do(method, path, body string, mutate ...func(*http.Request)) *httptest.ResponseRecorder {
	h.t.Helper()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	request.Header.Set("X-Real-IP", "203.0.113.7")
	for _, apply := range mutate {
		apply(request)
	}
	recorder := httptest.NewRecorder()
	h.handler.ServeHTTP(recorder, request)
	return recorder
}

func (h *harness) bearer(role auth.Role) func(*http.Request) {
	token, _, err := h.tokens.Issue("user-1", role)
	if err != nil {
		h.t.Fatalf("Issue returned error: %v", err)
	}
	return func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+token) }
}

type apiBody struct {
	Data  json.RawMessage `json:"data"`
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Details []struct {
			Field string `json:"field"`
			Issue string `json:"issue"`
		} `json:"details"`
	} `json:"error"`
	Meta struct {
		RequestID string `json:"requestId"`
		Timestamp string `json:"timestamp"`
	} `json:"meta"`
}

func decode(t *testing.T, recorder *httptest.ResponseRecorder) apiBody {
	t.Helper()
	var body apiBody
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not JSON: %v (%s)", err, recorder.Body.String())
	}
	return body
}

func withHeader(name, value string) func(*http.Request) {
	return func(r *http.Request) { r.Header.Set(name, value) }
}

const validRegisterBody = `{"email":"nimal@example.com","password":"Str0ngPass!","fullName":"Nimal Perera","phone":"+94771234567"}`

func TestAuthHandlers_register(t *testing.T) {
	t.Run("creates the account and returns the profile", func(t *testing.T) {
		h := newHarness(t)
		recorder := h.do(http.MethodPost, "/auth/register", validRegisterBody)

		if recorder.Code != http.StatusCreated {
			t.Fatalf("status = %d, want 201 (%s)", recorder.Code, recorder.Body.String())
		}
		body := decode(t, recorder)
		var data map[string]any
		if err := json.Unmarshal(body.Data, &data); err != nil {
			t.Fatalf("decode data: %v", err)
		}
		for _, key := range []string{"id", "email", "fullName", "phone", "role", "createdAt"} {
			if _, ok := data[key]; !ok {
				t.Fatalf("data lacks %q: %v", key, data)
			}
		}
		if data["role"] != "CUSTOMER" || data["createdAt"] != "2026-11-02T09:30:00Z" {
			t.Fatalf("data = %v", data)
		}
		if _, leaked := data["password"]; leaked {
			t.Fatal("the response contains a password field")
		}
		if body.Meta.RequestID == "" || body.Meta.Timestamp == "" {
			t.Fatalf("meta = %+v", body.Meta)
		}
		if len(recorder.Result().Cookies()) != 0 {
			t.Fatal("registering must not start a session")
		}
		if h.service.registered.Email != "nimal@example.com" || h.service.registered.Password != "Str0ngPass!" {
			t.Fatalf("service received %+v", h.service.registered)
		}
	})

	failures := []struct {
		name       string
		body       string
		contentTyp string
		err        error
		wantStatus int
		wantCode   string
		wantField  string
	}{
		{name: "malformed JSON", body: `{"email":`, wantStatus: 400, wantCode: "BAD_REQUEST"},
		{name: "empty body", body: ``, contentTyp: "application/json", wantStatus: 400, wantCode: "BAD_REQUEST"},
		{name: "wrong content type", body: validRegisterBody, contentTyp: "text/plain", wantStatus: 415, wantCode: "UNSUPPORTED_MEDIA_TYPE"},
		{name: "oversized body", body: `{"email":"` + strings.Repeat("a", maxBodyBytes) + `"}`, wantStatus: 413, wantCode: "PAYLOAD_TOO_LARGE"},
		{
			name: "validation errors list the fields",
			body: validRegisterBody,
			err: &service.ValidationError{Issues: []service.Issue{
				{Field: "password", Issue: "must be at least 8 characters"},
			}},
			wantStatus: 422, wantCode: "VALIDATION_ERROR", wantField: "password",
		},
		{name: "duplicate email", body: validRegisterBody, err: service.ErrEmailAlreadyExists, wantStatus: 409, wantCode: "EMAIL_ALREADY_EXISTS"},
	}
	for _, test := range failures {
		t.Run(test.name, func(t *testing.T) {
			h := newHarness(t)
			h.service.err = test.err

			recorder := h.do(http.MethodPost, "/auth/register", test.body, func(r *http.Request) {
				if test.contentTyp != "" {
					r.Header.Set("Content-Type", test.contentTyp)
				}
			})

			if recorder.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d (%s)", recorder.Code, test.wantStatus, recorder.Body.String())
			}
			body := decode(t, recorder)
			if body.Error.Code != test.wantCode {
				t.Fatalf("error code = %q, want %q", body.Error.Code, test.wantCode)
			}
			if test.wantField != "" && (len(body.Error.Details) != 1 || body.Error.Details[0].Field != test.wantField) {
				t.Fatalf("details = %+v, want field %q", body.Error.Details, test.wantField)
			}
		})
	}
}

func TestAuthHandlers_login(t *testing.T) {
	const loginBody = `{"email":"nimal@example.com","password":"Str0ngPass!"}`

	t.Run("sets both cookies and keeps tokens out of the body", func(t *testing.T) {
		h := newHarness(t)
		recorder := h.do(http.MethodPost, "/auth/login", loginBody, withHeader("User-Agent", "unit-test"))

		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (%s)", recorder.Code, recorder.Body.String())
		}
		access := cookieByName(t, recorder, "sd_access")
		refresh := cookieByName(t, recorder, "sd_refresh")
		if access.Value != "access-token-value" || refresh.Value != "rt_refresh-token-value" {
			t.Fatal("cookies do not carry the session tokens")
		}
		if !access.HttpOnly || !refresh.HttpOnly {
			t.Fatal("session cookies must be HttpOnly")
		}

		raw := recorder.Body.String()
		if strings.Contains(raw, "access-token-value") || strings.Contains(raw, "rt_refresh-token-value") {
			t.Fatalf("tokens leaked into the body: %s", raw)
		}
		var data struct {
			User struct {
				ID, Email, FullName, Role, Phone string
			} `json:"user"`
			ExpiresIn int `json:"expiresIn"`
		}
		if err := json.Unmarshal(decode(t, recorder).Data, &data); err != nil {
			t.Fatalf("decode data: %v", err)
		}
		if data.ExpiresIn != 900 || data.User.ID != "user-1" || data.User.Role != "CUSTOMER" || data.User.Phone != "" {
			t.Fatalf("data = %+v", data)
		}
		if recorder.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("Cache-Control = %q, want no-store", recorder.Header().Get("Cache-Control"))
		}
	})

	t.Run("passes the client address and agent to the service", func(t *testing.T) {
		h := newHarness(t)
		h.do(http.MethodPost, "/auth/login", loginBody, withHeader("User-Agent", "unit-test"))

		if h.service.loggedIn.UserAgent != "unit-test" || h.service.loggedIn.IP.String() != "203.0.113.7" {
			t.Fatalf("service received %+v", h.service.loggedIn)
		}
	})

	t.Run("token mode adds the tokens to the body", func(t *testing.T) {
		h := newHarness(t)
		recorder := h.do(http.MethodPost, "/auth/login", loginBody, withHeader("X-Token-Mode", "body"))

		var data struct {
			AccessToken  string `json:"accessToken"`
			RefreshToken string `json:"refreshToken"`
		}
		if err := json.Unmarshal(decode(t, recorder).Data, &data); err != nil {
			t.Fatalf("decode data: %v", err)
		}
		if data.AccessToken != "access-token-value" || data.RefreshToken != "rt_refresh-token-value" {
			t.Fatalf("data = %+v", data)
		}
		cookieByName(t, recorder, "sd_access") // cookies are still set in token mode
	})

	t.Run("missing fields are a validation error", func(t *testing.T) {
		h := newHarness(t)
		recorder := h.do(http.MethodPost, "/auth/login", `{"email":" ","password":""}`)

		if recorder.Code != http.StatusUnprocessableEntity {
			t.Fatalf("status = %d, want 422", recorder.Code)
		}
		if body := decode(t, recorder); len(body.Error.Details) != 2 {
			t.Fatalf("details = %+v, want email and password", body.Error.Details)
		}
		if h.service.calls != 0 {
			t.Fatal("the service was called for an empty login")
		}
	})

	failures := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{name: "wrong credentials", err: service.ErrInvalidCredentials, wantStatus: 401, wantCode: "INVALID_CREDENTIALS"},
		{name: "disabled account", err: service.ErrAccountDisabled, wantStatus: 403, wantCode: "ACCOUNT_DISABLED"},
		{name: "locked account", err: service.ErrAccountLocked, wantStatus: 423, wantCode: "ACCOUNT_LOCKED"},
	}
	for _, test := range failures {
		t.Run(test.name, func(t *testing.T) {
			h := newHarness(t)
			h.service.err = test.err

			recorder := h.do(http.MethodPost, "/auth/login", loginBody)
			if recorder.Code != test.wantStatus || decode(t, recorder).Error.Code != test.wantCode {
				t.Fatalf("got %d %s, want %d %s", recorder.Code, recorder.Body.String(), test.wantStatus, test.wantCode)
			}
			if len(recorder.Result().Cookies()) != 0 {
				t.Fatal("a failed login set cookies")
			}
		})
	}

	t.Run("an unexpected error is a generic 500 and is logged without the secret", func(t *testing.T) {
		h := newHarness(t)
		h.service.err = errors.New("dial tcp 10.0.0.5:5432: connection refused")

		recorder := h.do(http.MethodPost, "/auth/login", loginBody)
		if recorder.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want 500", recorder.Code)
		}
		body := decode(t, recorder)
		if body.Error.Code != "INTERNAL_ERROR" || strings.Contains(recorder.Body.String(), "10.0.0.5") {
			t.Fatalf("body = %s", recorder.Body.String())
		}
		logged := h.logs.String()
		if !strings.Contains(logged, "auth request failed") || strings.Contains(logged, "Str0ngPass!") {
			t.Fatalf("log output = %q", logged)
		}
	})
}

func TestAuthHandlers_loginRateLimit(t *testing.T) {
	h := newHarness(t)
	const body = `{"email":"nimal@example.com","password":"Str0ngPass!"}`

	for attempt := 1; attempt <= authAttemptsPerMinute; attempt++ {
		if recorder := h.do(http.MethodPost, "/auth/login", body); recorder.Code != http.StatusOK {
			t.Fatalf("attempt %d status = %d, want 200", attempt, recorder.Code)
		}
	}

	t.Run("the next attempt is limited, even with a forged forwarding header", func(t *testing.T) {
		recorder := h.do(http.MethodPost, "/auth/login", body, withHeader("X-Forwarded-For", "198.51.100.99"))
		if recorder.Code != http.StatusTooManyRequests {
			t.Fatalf("status = %d, want 429", recorder.Code)
		}
		if decode(t, recorder).Error.Code != "RATE_LIMITED" || recorder.Header().Get("Retry-After") == "" {
			t.Fatalf("429 response lacks RATE_LIMITED or Retry-After: %s", recorder.Body.String())
		}
	})

	t.Run("registration shares the limit", func(t *testing.T) {
		if recorder := h.do(http.MethodPost, "/auth/register", validRegisterBody); recorder.Code != http.StatusTooManyRequests {
			t.Fatalf("status = %d, want 429", recorder.Code)
		}
	})

	t.Run("another client address is not limited", func(t *testing.T) {
		recorder := h.do(http.MethodPost, "/auth/login", body, withHeader("X-Real-IP", "203.0.113.8"))
		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", recorder.Code)
		}
	})

	t.Run("refresh is not part of the login limit", func(t *testing.T) {
		recorder := h.do(http.MethodPost, "/auth/refresh", "", func(r *http.Request) {
			r.AddCookie(&http.Cookie{Name: "sd_refresh", Value: "rt_value"})
		})
		if recorder.Code == http.StatusTooManyRequests {
			t.Fatal("refresh was rate limited with the login group")
		}
	})
}

func TestAuthHandlers_refresh(t *testing.T) {
	t.Run("uses the refresh cookie and rotates both cookies", func(t *testing.T) {
		h := newHarness(t)
		recorder := h.do(http.MethodPost, "/auth/refresh", "", func(r *http.Request) {
			r.AddCookie(&http.Cookie{Name: "sd_refresh", Value: "rt_old-token"})
		})

		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (%s)", recorder.Code, recorder.Body.String())
		}
		if h.service.refreshed.RefreshToken != "rt_old-token" {
			t.Fatalf("service received %+v", h.service.refreshed)
		}
		if cookieByName(t, recorder, "sd_refresh").Value != "rt_refresh-token-value" {
			t.Fatal("the refresh cookie was not rotated")
		}
		if strings.Contains(recorder.Body.String(), "access-token-value") {
			t.Fatal("tokens leaked into the body outside token mode")
		}
		var data map[string]any
		_ = json.Unmarshal(decode(t, recorder).Data, &data)
		if _, hasUser := data["user"]; hasUser || data["expiresIn"] != float64(900) {
			t.Fatalf("data = %v, want only expiresIn", data)
		}
	})

	t.Run("token mode reads the body and returns the tokens", func(t *testing.T) {
		h := newHarness(t)
		recorder := h.do(http.MethodPost, "/auth/refresh", `{"refreshToken":"rt_body-token"}`, withHeader("X-Token-Mode", "body"))

		if recorder.Code != http.StatusOK || h.service.refreshed.RefreshToken != "rt_body-token" {
			t.Fatalf("status %d, service received %+v", recorder.Code, h.service.refreshed)
		}
		if !strings.Contains(recorder.Body.String(), "rt_refresh-token-value") {
			t.Fatal("token mode did not return the new refresh token")
		}
	})

	t.Run("a cookie is ignored in token mode", func(t *testing.T) {
		h := newHarness(t)
		h.do(http.MethodPost, "/auth/refresh", `{"refreshToken":"rt_body-token"}`, withHeader("X-Token-Mode", "body"), func(r *http.Request) {
			r.AddCookie(&http.Cookie{Name: "sd_refresh", Value: "rt_cookie-token"})
		})
		if h.service.refreshed.RefreshToken != "rt_body-token" {
			t.Fatalf("service received %+v, want the body token", h.service.refreshed)
		}
	})

	t.Run("an invalid token is 401 and clears the cookies", func(t *testing.T) {
		h := newHarness(t)
		h.service.err = service.ErrRefreshTokenInvalid

		recorder := h.do(http.MethodPost, "/auth/refresh", "")
		if recorder.Code != http.StatusUnauthorized || decode(t, recorder).Error.Code != "REFRESH_TOKEN_INVALID" {
			t.Fatalf("got %d %s", recorder.Code, recorder.Body.String())
		}
		if cookie := cookieByName(t, recorder, "sd_refresh"); cookie.MaxAge >= 0 {
			t.Fatalf("the refresh cookie was not cleared: %+v", cookie)
		}
	})

	t.Run("token mode with a malformed body is a 400", func(t *testing.T) {
		h := newHarness(t)
		recorder := h.do(http.MethodPost, "/auth/refresh", `{`, withHeader("X-Token-Mode", "body"))
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", recorder.Code)
		}
	})
}

func TestAuthHandlers_logout(t *testing.T) {
	t.Run("requires an access token", func(t *testing.T) {
		h := newHarness(t)
		recorder := h.do(http.MethodPost, "/auth/logout", "")

		if recorder.Code != http.StatusUnauthorized || decode(t, recorder).Error.Code != "UNAUTHENTICATED" {
			t.Fatalf("got %d %s", recorder.Code, recorder.Body.String())
		}
		if h.service.calls != 0 {
			t.Fatal("logout reached the service without authentication")
		}
	})

	t.Run("revokes the caller's own refresh token and clears the cookies", func(t *testing.T) {
		h := newHarness(t)
		recorder := h.do(http.MethodPost, "/auth/logout", "", h.bearer(auth.RoleCustomer), func(r *http.Request) {
			r.AddCookie(&http.Cookie{Name: "sd_refresh", Value: "rt_current"})
		})

		if recorder.Code != http.StatusNoContent || recorder.Body.Len() != 0 {
			t.Fatalf("got %d with body %q, want an empty 204", recorder.Code, recorder.Body.String())
		}
		if h.service.logoutUser != "user-1" || h.service.logoutTok != "rt_current" {
			t.Fatalf("service received user %q token %q", h.service.logoutUser, h.service.logoutTok)
		}
		if cookieByName(t, recorder, "sd_access").MaxAge >= 0 || cookieByName(t, recorder, "sd_refresh").MaxAge >= 0 {
			t.Fatal("logout did not clear both cookies")
		}
	})

	t.Run("works without a refresh cookie", func(t *testing.T) {
		h := newHarness(t)
		recorder := h.do(http.MethodPost, "/auth/logout", "", h.bearer(auth.RoleCourier))

		if recorder.Code != http.StatusNoContent || h.service.logoutTok != "" {
			t.Fatalf("status %d, token %q", recorder.Code, h.service.logoutTok)
		}
	})

	t.Run("token mode may send the refresh token in the body", func(t *testing.T) {
		h := newHarness(t)
		h.do(http.MethodPost, "/auth/logout", `{"refreshToken":"rt_from_body"}`, h.bearer(auth.RoleCustomer), withHeader("X-Token-Mode", "body"))

		if h.service.logoutTok != "rt_from_body" {
			t.Fatalf("logout token = %q, want rt_from_body", h.service.logoutTok)
		}
	})

	t.Run("a service failure is a generic 500", func(t *testing.T) {
		h := newHarness(t)
		h.service.err = errors.New("boom")

		recorder := h.do(http.MethodPost, "/auth/logout", "", h.bearer(auth.RoleCustomer))
		if recorder.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want 500", recorder.Code)
		}
	})
}

func TestAuthHandlers_routing(t *testing.T) {
	h := newHarness(t)

	t.Run("a wrong method is 405 JSON", func(t *testing.T) {
		recorder := h.do(http.MethodGet, "/auth/login", "")
		if recorder.Code != http.StatusMethodNotAllowed || decode(t, recorder).Error.Code != "METHOD_NOT_ALLOWED" {
			t.Fatalf("got %d %s", recorder.Code, recorder.Body.String())
		}
	})

	t.Run("an unknown auth path is 404 JSON", func(t *testing.T) {
		recorder := h.do(http.MethodPost, "/auth/unknown", "")
		if recorder.Code != http.StatusNotFound || decode(t, recorder).Error.Code != "NOT_FOUND" {
			t.Fatalf("got %d %s", recorder.Code, recorder.Body.String())
		}
	})

	t.Run("no auth routes exist without WithAuth", func(t *testing.T) {
		bare := New(slog.New(slog.NewTextHandler(io.Discard, nil)))
		recorder := httptest.NewRecorder()
		bare.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/auth/login", nil))
		if recorder.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", recorder.Code)
		}
	})
}
