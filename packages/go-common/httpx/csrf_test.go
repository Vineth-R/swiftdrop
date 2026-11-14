package httpx

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func csrfRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(CrossOriginMiddleware())
	router.POST("/mutate", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	router.GET("/read", func(c *gin.Context) { c.Status(http.StatusOK) })
	return router
}

func csrfRequest(t *testing.T, method, path string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, nil)
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	if _, forwarded := headers["X-Forwarded-Host"]; forwarded && headers["X-Real-IP"] == "" {
		request.Header.Set("X-Real-IP", "203.0.113.10")
	}
	recorder := httptest.NewRecorder()
	csrfRouter().ServeHTTP(recorder, request)
	return recorder
}

func TestCrossOriginAllowsSafeMethods(t *testing.T) {
	recorder := csrfRequest(t, http.MethodGet, "/read", map[string]string{"Sec-Fetch-Site": "cross-site"})
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
}

func TestCrossOriginRejectsCrossSiteMutation(t *testing.T) {
	recorder := csrfRequest(t, http.MethodPost, "/mutate", map[string]string{"Sec-Fetch-Site": "cross-site"})
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403: %s", recorder.Code, recorder.Body.String())
	}
	if recorder.Header().Get("Content-Type") != "application/json; charset=utf-8" {
		t.Fatalf("unexpected content type %q", recorder.Header().Get("Content-Type"))
	}
}

func TestCrossOriginRejectsMismatchedOrigin(t *testing.T) {
	recorder := csrfRequest(t, http.MethodPost, "/mutate", map[string]string{
		"Origin":            "https://evil.example",
		"X-Forwarded-Host":  "swiftdrop.example",
		"X-Forwarded-Proto": "https",
	})
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", recorder.Code)
	}
}

func TestCrossOriginAcceptsMatchingOrigin(t *testing.T) {
	recorder := csrfRequest(t, http.MethodPost, "/mutate", map[string]string{
		"Origin":            "https://swiftdrop.example",
		"X-Forwarded-Host":  "swiftdrop.example",
		"X-Forwarded-Proto": "https",
	})
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204: %s", recorder.Code, recorder.Body.String())
	}
}

func TestCrossOriginAcceptsFirstForwardedValues(t *testing.T) {
	recorder := csrfRequest(t, http.MethodPost, "/mutate", map[string]string{
		"Origin":            "http://localhost:8080",
		"X-Forwarded-Host":  "localhost:8080, internal",
		"X-Forwarded-Proto": "http, http",
	})
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", recorder.Code)
	}
}

func TestCrossOriginAllowsLoopbackDevelopmentProxy(t *testing.T) {
	recorder := csrfRequest(t, http.MethodPost, "/mutate", map[string]string{
		"Origin":           "http://127.0.0.1:3000",
		"X-Forwarded-Host": "127.0.0.1:8080",
		"X-Real-IP":        "203.0.113.10",
	})
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", recorder.Code)
	}
}

func TestCrossOriginDoesNotTreatRemoteHostAsLoopback(t *testing.T) {
	recorder := csrfRequest(t, http.MethodPost, "/mutate", map[string]string{
		"Origin":           "http://127.0.0.1:3000",
		"X-Forwarded-Host": "swiftdrop.example",
		"X-Real-IP":        "203.0.113.10",
	})
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", recorder.Code)
	}
}

func TestCrossOriginWithoutForwardedHostSkipsOriginComparison(t *testing.T) {
	// A direct local call (the Next.js dev proxy) has no forwarded host. It is
	// still subject to the browser's Sec-Fetch-Site signal, checked separately.
	recorder := csrfRequest(t, http.MethodPost, "/mutate", map[string]string{"Origin": "http://localhost:3000"})
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", recorder.Code)
	}
}

func TestCrossOriginRejectsNullOrigin(t *testing.T) {
	recorder := csrfRequest(t, http.MethodPost, "/mutate", map[string]string{
		"Origin":           "null",
		"X-Forwarded-Host": "swiftdrop.example",
		"X-Real-IP":        "203.0.113.10",
	})
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", recorder.Code)
	}
}

func TestCrossOriginRejectsMalformedOrigin(t *testing.T) {
	recorder := csrfRequest(t, http.MethodPost, "/mutate", map[string]string{
		"Origin":           "not a url",
		"X-Forwarded-Host": "swiftdrop.example",
		"X-Real-IP":        "203.0.113.10",
	})
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", recorder.Code)
	}
}
