package servicehttp

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"
	"time"

	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
)

// TestRouter_everyOptionTogether mounts all the identity features on one router,
// as main does. Gin panics on conflicting routes, and a pair of options that
// each work alone can still collide, so this is checked explicitly.
func TestRouter_everyOptionTogether(t *testing.T) {
	tokens, err := auth.NewTokenManager(handlerTestSecret, 15*time.Minute)
	if err != nil {
		t.Fatalf("NewTokenManager returned error: %v", err)
	}
	cookies := CookieConfig{AccessName: "sd_access", RefreshName: "sd_refresh", SameSite: http.SameSiteLaxMode}

	router := New(slog.New(slog.DiscardHandler),
		WithAuth(AuthRoutes{Service: &fakeService{}, Verifier: tokens, Cookies: cookies}),
		WithGoogle(GoogleRoutes{Service: &fakeGoogleService{}, Cookies: cookies}),
		WithPasswordReset(PasswordResetRoutes{Service: &fakeResetService{}, Cookies: cookies}),
		WithInternal(InternalRoutes{Service: &fakeInternalService{}, Token: testInternalToken}),
		WithAdminUsers(AdminRoutes{Service: &fakeAdminService{}, Verifier: tokens, Cookies: cookies}),
	)

	var got []string
	for _, route := range router.Routes() {
		got = append(got, route.Method+" "+route.Path)
	}
	sort.Strings(got)

	want := []string{
		"GET /admin/users", "GET /admin/users/", "GET /admin/users/:userId",
		"GET /auth/google/callback", "GET /auth/google/login",
		"GET /couriers", "GET /couriers/",
		"GET /internal/couriers/:courierId/eligibility", "GET /internal/users/:userId/contact",
		"PATCH /admin/users/:userId", "PATCH /admin/users/:userId/status",
		"POST /admin/users", "POST /admin/users/",
		"POST /auth/forgot-password", "POST /auth/login", "POST /auth/logout", "POST /auth/refresh",
		"POST /auth/register", "POST /auth/reset-password",
	}
	have := map[string]bool{}
	for _, route := range got {
		have[route] = true
	}
	for _, route := range want {
		if !have[route] {
			t.Errorf("route %q is not registered", route)
		}
	}

	// PUT /users/me/password belongs to M2 and must not be claimed here.
	for _, route := range got {
		if route == "PUT /users/me/password" {
			t.Errorf("route %q must not be registered by M3", route)
		}
	}

	// A request through the full router still reaches the right handler.
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/admin/users", nil))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("GET /admin/users without credentials = %d, want 401", recorder.Code)
	}
}
