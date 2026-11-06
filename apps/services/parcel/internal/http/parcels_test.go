package servicehttp

import (
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
)

func TestParcelRoutesFailClosed(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	for _, secret := range []string{"", strings.Repeat("s", 32)} {
		router, err := New(logger, Options{AccessSecret: secret, InternalToken: "test-internal-token"})
		if err != nil {
			t.Fatal(err)
		}
		for _, path := range []string{"/parcels", "/parcels/11111111-1111-4111-8111-111111111111", "/parcels/11111111-1111-4111-8111-111111111111/history", "/parcels/11111111-1111-4111-8111-111111111111/receipt"} {
			req := httptest.NewRequest("GET", path, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			want := 401
			if secret == "" {
				want = 503
			}
			if w.Code != want {
				t.Fatalf("%s secret configured %v: %d %s", path, secret != "", w.Code, w.Body.String())
			}
			if w.Header().Get("Cache-Control") != "no-store" {
				t.Error("missing no-store")
			}
		}
		for _, token := range []string{"", "wrong"} {
			req := httptest.NewRequest("GET", "/internal/parcels/11111111-1111-4111-8111-111111111111/summary", nil)
			req.Header.Set("X-Internal-Token", token)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			if w.Code != 401 {
				t.Fatalf("internal accepted token: %d", w.Code)
			}
		}
	}
}
func TestParcelRolesValidationAndUnavailable(t *testing.T) {
	secret := strings.Repeat("s", 32)
	manager, err := auth.NewTokenManager(secret, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	router, err := New(logger, Options{AccessSecret: secret})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		role               auth.Role
		method, path, body string
		want               int
	}{{auth.RoleCourier, "GET", "/parcels", "", 403}, {auth.RoleDispatcher, "POST", "/parcels", `{}`, 403}, {auth.RoleDispatcher, "GET", "/parcels/11111111-1111-4111-8111-111111111111/receipt", "", 403}, {auth.RoleCustomer, "POST", "/parcels", `{}`, 422}, {auth.RoleCustomer, "POST", "/parcels", `{"unknown":true}`, 400}, {auth.RoleCustomer, "POST", "/parcels", `{broken}`, 400}, {auth.RoleCustomer, "GET", "/parcels?page=wrong", "", 422}, {auth.RoleCustomer, "GET", "/parcels?from=tomorrow", "", 422}, {auth.RoleCustomer, "GET", "/parcels?pageSize=101", "", 422}, {auth.RoleCustomer, "GET", "/parcels", "", 503}, {auth.RoleCustomer, "GET", "/parcels/not-uuid", "", 422}, {auth.RoleAdmin, "GET", "/parcels/11111111-1111-4111-8111-111111111111", "", 503}} {
		token, _, err := manager.Issue("11111111-1111-4111-8111-111111111111", tc.role)
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != tc.want {
			t.Fatalf("%s %s %s: want %d got %d %s", tc.role, tc.method, tc.path, tc.want, w.Code, w.Body.String())
		}
	}
}
