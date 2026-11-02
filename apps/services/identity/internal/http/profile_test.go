package servicehttp

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Vineth-R/swiftdrop/apps/services/identity/internal/service"
	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
)

const profileUserID = "3f1c7a52-0000-4000-8000-0000000000c3"

type fakeProfileService struct {
	profile service.Profile
	err     error
	calls   int
	gotID   string
}

func (f *fakeProfileService) Profile(_ context.Context, userID string) (service.Profile, error) {
	f.calls++
	f.gotID = userID
	return f.profile, f.err
}

type profileHarness struct {
	service *fakeProfileService
	handler http.Handler
	tokens  *auth.TokenManager
}

func newProfileHarness(t *testing.T) *profileHarness {
	t.Helper()
	tokens, err := auth.NewTokenManager(handlerTestSecret, 15*time.Minute)
	if err != nil {
		t.Fatalf("NewTokenManager returned error: %v", err)
	}
	fake := &fakeProfileService{profile: service.Profile{
		ID:           profileUserID,
		Email:        "nimal@example.com",
		FullName:     "Nimal Perera",
		Phone:        "+94771234567",
		Role:         auth.RoleCustomer,
		AuthProvider: "LOCAL",
		CreatedAt:    time.Date(2026, time.November, 2, 9, 30, 0, 0, time.UTC),
	}}
	handler := New(slog.New(slog.DiscardHandler), WithProfile(ProfileRoutes{
		Service:  fake,
		Verifier: tokens,
		Cookies:  CookieConfig{AccessName: "sd_access", RefreshName: "sd_refresh"},
	}))
	return &profileHarness{service: fake, handler: handler, tokens: tokens}
}

func (h *profileHarness) token(t *testing.T, userID string, role auth.Role) string {
	t.Helper()
	token, _, err := h.tokens.Issue(userID, role)
	if err != nil {
		t.Fatalf("Issue returned error: %v", err)
	}
	return token
}

func (h *profileHarness) get(t *testing.T, path, bearer string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, path, nil)
	if bearer != "" {
		request.Header.Set("Authorization", "Bearer "+bearer)
	}
	recorder := httptest.NewRecorder()
	h.handler.ServeHTTP(recorder, request)
	return recorder
}

func TestProfile_requiresAuthentication(t *testing.T) {
	h := newProfileHarness(t)

	response := h.get(t, "/users/me", "")

	if response.Code != http.StatusUnauthorized || errorCode(t, response) != "UNAUTHENTICATED" {
		t.Fatalf("status = %d body = %s, want 401 UNAUTHENTICATED", response.Code, response.Body)
	}
	if h.service.calls != 0 {
		t.Fatal("an unauthenticated request reached the service")
	}
}

func TestProfile_returnsTheSignedInUsersAccount(t *testing.T) {
	h := newProfileHarness(t)

	response := h.get(t, "/users/me", h.token(t, profileUserID, auth.RoleCustomer))

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s, want 200", response.Code, response.Body)
	}
	var body struct {
		Data map[string]any `json:"data"`
		Meta struct {
			RequestID string `json:"requestId"`
			Timestamp string `json:"timestamp"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not JSON: %v", err)
	}

	want := map[string]any{
		"id":           profileUserID,
		"email":        "nimal@example.com",
		"fullName":     "Nimal Perera",
		"phone":        "+94771234567",
		"role":         "CUSTOMER",
		"authProvider": "LOCAL",
		"createdAt":    "2026-11-02T09:30:00Z",
	}
	if len(body.Data) != len(want) {
		t.Fatalf("data has %d fields, want exactly %d: %v", len(body.Data), len(want), body.Data)
	}
	for key, value := range want {
		if body.Data[key] != value {
			t.Errorf("data.%s = %v, want %v", key, body.Data[key], value)
		}
	}
	if body.Meta.RequestID == "" || body.Meta.Timestamp == "" {
		t.Errorf("meta = %+v, want a request ID and a timestamp", body.Meta)
	}
	if got := response.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", got)
	}
	for _, secret := range []string{"password", "hash", "token"} {
		if strings.Contains(strings.ToLower(response.Body.String()), secret) {
			t.Errorf("response mentions %q: %s", secret, response.Body)
		}
	}
}

func TestProfile_readsTheUserFromTheTokenOnly(t *testing.T) {
	h := newProfileHarness(t)

	response := h.get(t, "/users/me?id=3f1c7a52-0000-4000-8000-0000000000ff&userId=other",
		h.token(t, profileUserID, auth.RoleCustomer))

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.Code)
	}
	if h.service.gotID != profileUserID {
		t.Fatalf("service was asked for %q, want the user in the token %q", h.service.gotID, profileUserID)
	}
}

func TestProfile_everyRoleMayReadItsOwnAccount(t *testing.T) {
	h := newProfileHarness(t)

	for _, role := range []auth.Role{auth.RoleCustomer, auth.RoleMerchant, auth.RoleCourier, auth.RoleDispatcher, auth.RoleAdmin} {
		if response := h.get(t, "/users/me", h.token(t, profileUserID, role)); response.Code != http.StatusOK {
			t.Fatalf("%s status = %d, want 200", role, response.Code)
		}
	}
}

func TestProfile_acceptsTheSessionCookie(t *testing.T) {
	h := newProfileHarness(t)
	request := httptest.NewRequest(http.MethodGet, "/users/me", nil)
	request.AddCookie(&http.Cookie{Name: "sd_access", Value: h.token(t, profileUserID, auth.RoleCustomer)})
	recorder := httptest.NewRecorder()

	h.handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 with the access cookie", recorder.Code)
	}
}

func TestProfile_refusesAccountsThatAreGoneOrDisabled(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		status   int
		wantCode string
	}{
		{"the account no longer exists", service.ErrUserNotFound, http.StatusUnauthorized, "UNAUTHENTICATED"},
		{"the account was deactivated", service.ErrAccountDisabled, http.StatusForbidden, "ACCOUNT_DISABLED"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			h := newProfileHarness(t)
			h.service.err = test.err

			response := h.get(t, "/users/me", h.token(t, profileUserID, auth.RoleCustomer))

			if response.Code != test.status || errorCode(t, response) != test.wantCode {
				t.Fatalf("status = %d body = %s, want %d %s", response.Code, response.Body, test.status, test.wantCode)
			}
		})
	}
}

func TestProfile_serverErrorsAreGeneric(t *testing.T) {
	h := newProfileHarness(t)
	h.service.err = errors.New("connection to db.internal:5432 refused")

	response := h.get(t, "/users/me", h.token(t, profileUserID, auth.RoleCustomer))

	if response.Code != http.StatusInternalServerError || errorCode(t, response) != "INTERNAL_ERROR" {
		t.Fatalf("status = %d body = %s, want 500 INTERNAL_ERROR", response.Code, response.Body)
	}
	if strings.Contains(response.Body.String(), "db.internal") {
		t.Fatalf("the response leaks the underlying error: %s", response.Body)
	}
}

func TestProfile_isLimitedPerUser(t *testing.T) {
	h := newProfileHarness(t)
	token := h.token(t, profileUserID, auth.RoleCustomer)

	var last *httptest.ResponseRecorder
	for range userRequestsPerMinute + 1 {
		last = h.get(t, "/users/me", token)
	}

	if last.Code != http.StatusTooManyRequests || errorCode(t, last) != "RATE_LIMITED" {
		t.Fatalf("request %d status = %d, want 429 RATE_LIMITED", userRequestsPerMinute+1, last.Code)
	}
	if last.Header().Get("Retry-After") == "" {
		t.Error("a rate limited response must carry Retry-After")
	}

	other := h.get(t, "/users/me", h.token(t, "3f1c7a52-0000-4000-8000-0000000000d4", auth.RoleCustomer))
	if other.Code != http.StatusOK {
		t.Fatalf("another user was limited too: status = %d", other.Code)
	}
}
