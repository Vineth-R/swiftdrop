package servicehttp

import (
	"bytes"
	"errors"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

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

// TestParcelStateChangingRequestsRequireJSON documents the cookie-based CSRF
// posture: session cookies are HttpOnly and SameSite=Lax, and state-changing
// parcel routes reject anything that is not application/json. A cross-site form
// can send text/plain or urlencoded data without a CORS preflight but cannot
// produce this content type, so the request is refused before any handler runs.
func TestParcelStateChangingRequestsRequireJSON(t *testing.T) {
	secret := strings.Repeat("s", 32)
	manager, err := auth.NewTokenManager(secret, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := manager.Issue("11111111-1111-4111-8111-111111111111", auth.RoleCustomer)
	if err != nil {
		t.Fatal(err)
	}
	router, err := New(slog.New(slog.NewTextHandler(io.Discard, nil)), Options{AccessSecret: secret})
	if err != nil {
		t.Fatal(err)
	}
	body := `{"sender":{"name":"Sender"},"recipient":{"name":"Recipient"}}`
	for _, contentType := range []string{"", "text/plain", "application/x-www-form-urlencoded", "multipart/form-data"} {
		req := httptest.NewRequest("POST", "/parcels", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != 400 {
			t.Fatalf("content type %q: want 400, got %d %s", contentType, w.Code, w.Body.String())
		}
	}
}

func TestParcelInternalErrorsAreLoggedButNotExposed(t *testing.T) {
	var logs bytes.Buffer
	handler := &parcelHandlers{logger: slog.New(slog.NewJSONHandler(&logs, nil))}
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest("POST", "/parcels", nil)

	const detail = "database failure: constraint parcels_customer_idem_uq"
	if !handler.failure(c, errors.New(detail)) {
		t.Fatal("failure() did not report an error")
	}
	if recorder.Code != 500 {
		t.Fatalf("status = %d, want 500", recorder.Code)
	}
	if strings.Contains(recorder.Body.String(), "parcels_customer_idem_uq") {
		t.Fatal("internal error detail leaked to the client")
	}
	text := logs.String()
	if !strings.Contains(text, "parcel operation failed") || !strings.Contains(text, detail) || !strings.Contains(text, "request_id") {
		t.Fatalf("structured failure log is missing context: %s", text)
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
