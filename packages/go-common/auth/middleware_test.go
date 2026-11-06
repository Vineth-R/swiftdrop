package auth

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Vineth-R/swiftdrop/packages/go-common/httpx"
)

const testCookie = "sd_access"

func newTestRouter(t *testing.T, manager *TokenManager) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)

	router := httpx.NewRouter(slog.New(slog.NewTextHandler(io.Discard, nil)))
	router.GET("/me", Authenticate(manager, testCookie), func(c *gin.Context) {
		claims, ok := ClaimsFrom(c)
		if !ok {
			c.Status(http.StatusInternalServerError)
			return
		}
		c.JSON(http.StatusOK, gin.H{"sub": claims.UserID, "role": claims.Role})
	})
	router.GET("/dispatch", Authenticate(manager, testCookie), RequireRoles(RoleDispatcher), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})
	router.GET("/staff", Authenticate(manager, testCookie), RequireRoles(RoleCourier, RoleMerchant), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})
	router.GET("/nobody", Authenticate(manager, testCookie), RequireRoles(), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})
	router.GET("/unprotected-guard", RequireRoles(RoleAdmin), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})
	return router
}

func errorCode(t *testing.T, recorder *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not an error envelope: %v (%s)", err, recorder.Body.String())
	}
	return body.Error.Code
}

func TestAuthenticate(t *testing.T) {
	manager := newTestManager(t, time.Now())
	router := newTestRouter(t, manager)

	token, _, err := manager.Issue("user-1", RoleCustomer)
	if err != nil {
		t.Fatalf("Issue returned error: %v", err)
	}
	expiredManager := newTestManager(t, time.Now().Add(-time.Hour))
	expired, _, err := expiredManager.Issue("user-1", RoleCustomer)
	if err != nil {
		t.Fatalf("Issue returned error: %v", err)
	}

	tests := []struct {
		name       string
		header     string
		cookie     string
		wantStatus int
		wantCode   string
	}{
		{name: "bearer token", header: "Bearer " + token, wantStatus: http.StatusOK},
		{name: "bearer scheme is case insensitive", header: "bearer " + token, wantStatus: http.StatusOK},
		{name: "cookie token", cookie: token, wantStatus: http.StatusOK},
		{name: "no credentials", wantStatus: http.StatusUnauthorized, wantCode: "UNAUTHENTICATED"},
		{name: "wrong scheme", header: "Basic " + token, wantStatus: http.StatusUnauthorized, wantCode: "UNAUTHENTICATED"},
		{name: "empty bearer", header: "Bearer ", wantStatus: http.StatusUnauthorized, wantCode: "UNAUTHENTICATED"},
		{name: "bare header value", header: token, wantStatus: http.StatusUnauthorized, wantCode: "UNAUTHENTICATED"},
		{
			name:       "bad header does not fall back to a valid cookie",
			header:     "Basic abc",
			cookie:     token,
			wantStatus: http.StatusUnauthorized,
			wantCode:   "UNAUTHENTICATED",
		},
		{name: "tampered token", header: "Bearer " + token + "x", wantStatus: http.StatusUnauthorized, wantCode: "UNAUTHENTICATED"},
		{name: "expired token", header: "Bearer " + expired, wantStatus: http.StatusUnauthorized, wantCode: "TOKEN_EXPIRED"},
		{name: "expired cookie", cookie: expired, wantStatus: http.StatusUnauthorized, wantCode: "TOKEN_EXPIRED"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/me", nil)
			if test.header != "" {
				request.Header.Set("Authorization", test.header)
			}
			if test.cookie != "" {
				request.AddCookie(&http.Cookie{Name: testCookie, Value: test.cookie})
			}

			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)

			if recorder.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d (%s)", recorder.Code, test.wantStatus, recorder.Body.String())
			}
			if test.wantCode != "" {
				if got := errorCode(t, recorder); got != test.wantCode {
					t.Fatalf("error code = %q, want %q", got, test.wantCode)
				}
			}
		})
	}
}

func TestAuthenticate_exposesClaimsToHandlers(t *testing.T) {
	manager := newTestManager(t, time.Now())
	router := newTestRouter(t, manager)

	token, _, err := manager.Issue("user-42", RoleMerchant)
	if err != nil {
		t.Fatalf("Issue returned error: %v", err)
	}

	request := httptest.NewRequest(http.MethodGet, "/me", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	var body struct {
		Sub  string `json:"sub"`
		Role string `json:"role"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Sub != "user-42" || body.Role != "MERCHANT" {
		t.Fatalf("handler saw %+v, want user-42 MERCHANT", body)
	}
}

func TestRequireRoles(t *testing.T) {
	manager := newTestManager(t, time.Now())
	router := newTestRouter(t, manager)

	tests := []struct {
		name       string
		role       Role
		path       string
		wantStatus int
	}{
		{name: "dispatcher reaches dispatch", role: RoleDispatcher, path: "/dispatch", wantStatus: http.StatusNoContent},
		{name: "admin reaches dispatch", role: RoleAdmin, path: "/dispatch", wantStatus: http.StatusNoContent},
		{name: "courier is refused dispatch", role: RoleCourier, path: "/dispatch", wantStatus: http.StatusForbidden},
		{name: "customer is refused dispatch", role: RoleCustomer, path: "/dispatch", wantStatus: http.StatusForbidden},
		{name: "any listed role is enough", role: RoleMerchant, path: "/staff", wantStatus: http.StatusNoContent},
		{name: "admin is not a courier", role: RoleAdmin, path: "/staff", wantStatus: http.StatusForbidden},
		{name: "empty role list denies everyone", role: RoleAdmin, path: "/nobody", wantStatus: http.StatusForbidden},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			token, _, err := manager.Issue("user-1", test.role)
			if err != nil {
				t.Fatalf("Issue returned error: %v", err)
			}

			request := httptest.NewRequest(http.MethodGet, test.path, nil)
			request.Header.Set("Authorization", "Bearer "+token)
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)

			if recorder.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d", recorder.Code, test.wantStatus)
			}
			if test.wantStatus == http.StatusForbidden {
				if got := errorCode(t, recorder); got != "FORBIDDEN" {
					t.Fatalf("error code = %q, want FORBIDDEN", got)
				}
			}
		})
	}
}

func TestRequireRoles_withoutAuthenticateFailsClosed(t *testing.T) {
	router := newTestRouter(t, newTestManager(t, time.Now()))

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/unprotected-guard", nil))

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusInternalServerError)
	}
	if got := errorCode(t, recorder); got != "INTERNAL_ERROR" {
		t.Fatalf("error code = %q, want INTERNAL_ERROR", got)
	}
}
