package httpx

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

func TestRequireRole(t *testing.T) {
	const secret = "test-only-key-with-more-than-thirty-two-bytes"
	for _, test := range []struct {
		name, role, key, method string
		expired                 bool
		status                  int
	}{
		{name: "admin", role: "ADMIN", key: secret, method: "HS256", status: 204},
		{name: "wrong role", role: "CUSTOMER", key: secret, method: "HS256", status: 403},
		{name: "bad signature", role: "ADMIN", key: "different-secret", method: "HS256", status: 401},
		{name: "expired", role: "ADMIN", key: secret, method: "HS256", expired: true, status: 401},
		{name: "wrong algorithm", role: "ADMIN", key: secret, method: "HS384", status: 401},
	} {
		t.Run(test.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			router := gin.New()
			router.GET("/admin", RequireRole(secret, "sd_access", "ADMIN"), func(c *gin.Context) { c.Status(204) })
			expires := time.Now().Add(time.Minute)
			if test.expired {
				expires = time.Now().Add(-time.Minute)
			}
			claims := accessClaims{Role: test.role, RegisteredClaims: jwt.RegisteredClaims{Subject: "user", ID: "session", IssuedAt: jwt.NewNumericDate(time.Now().Add(-time.Minute)), ExpiresAt: jwt.NewNumericDate(expires)}}
			raw, err := jwt.NewWithClaims(jwt.GetSigningMethod(test.method), claims).SignedString([]byte(test.key))
			if err != nil {
				t.Fatal(err)
			}
			request := httptest.NewRequest("GET", "/admin", nil)
			request.Header.Set("Authorization", "Bearer "+raw)
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)
			if recorder.Code != test.status {
				t.Fatalf("status %d want %d", recorder.Code, test.status)
			}
		})
	}
}

func TestAccessClaimsAndCookie(t *testing.T) {
	const secret = "test-only-key-with-more-than-thirty-two-bytes"
	for _, test := range []struct {
		name       string
		change     func(*accessClaims)
		configured bool
		status     int
	}{
		{name: "cookie", configured: true, status: 204},
		{name: "missing configuration", status: 503},
		{name: "missing expiry", configured: true, status: 401, change: func(c *accessClaims) { c.ExpiresAt = nil }},
		{name: "missing subject", configured: true, status: 401, change: func(c *accessClaims) { c.Subject = "" }},
		{name: "missing session", configured: true, status: 401, change: func(c *accessClaims) { c.ID = "" }},
		{name: "missing issue time", configured: true, status: 401, change: func(c *accessClaims) { c.IssuedAt = nil }},
		{name: "future issue time", configured: true, status: 401, change: func(c *accessClaims) { c.IssuedAt = jwt.NewNumericDate(time.Now().Add(time.Hour)) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			claims := accessClaims{Role: "ADMIN", RegisteredClaims: jwt.RegisteredClaims{Subject: "user", ID: "session", IssuedAt: jwt.NewNumericDate(time.Now().Add(-time.Minute)), ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))}}
			if test.change != nil {
				test.change(&claims)
			}
			raw, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
			if err != nil {
				t.Fatal(err)
			}
			key := ""
			if test.configured {
				key = secret
			}
			router := gin.New()
			router.GET("/admin", RequireRole(key, "sd_access", "ADMIN"), func(c *gin.Context) { c.Status(204) })
			request := httptest.NewRequest("GET", "/admin", nil)
			request.AddCookie(&http.Cookie{Name: "sd_access", Value: raw})
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)
			if recorder.Code != test.status {
				t.Fatalf("status %d want %d", recorder.Code, test.status)
			}
		})
	}
}
func TestInternalTokenFailsClosed(t *testing.T) {
	for _, test := range []struct {
		name, secret, provided string
		status                 int
	}{
		{name: "missing configuration", status: 401}, {name: "wrong secret", secret: "internal", provided: "wrong", status: 401},
		{name: "valid", secret: "internal", provided: "internal", status: 204},
	} {
		t.Run(test.name, func(t *testing.T) {
			router := gin.New()
			router.GET("/internal", InternalToken(test.secret), func(c *gin.Context) { c.Status(204) })
			request := httptest.NewRequest("GET", "/internal", nil)
			request.Header.Set("X-Internal-Token", test.provided)
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)
			if recorder.Code != test.status {
				t.Fatalf("status %d want %d", recorder.Code, test.status)
			}
		})
	}
}
