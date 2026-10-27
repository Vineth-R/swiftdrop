package requestid

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestNewProducesUniqueValidIDs(t *testing.T) {
	first := New()
	second := New()

	if !Valid(first) {
		t.Fatalf("New() produced an invalid ID: %q", first)
	}
	if first == second {
		t.Fatal("New() produced duplicate IDs")
	}
}

func TestValid(t *testing.T) {
	valid := []string{"abc", "550e8400-e29b-41d4-a716-446655440000", "request_123.test:1"}
	for _, value := range valid {
		if !Valid(value) {
			t.Fatalf("Valid(%q) = false, want true", value)
		}
	}

	invalid := []string{"", " ", "has space", "line\nbreak", string(make([]byte, 129))}
	for _, value := range invalid {
		if Valid(value) {
			t.Fatalf("Valid(%q) = true, want false", value)
		}
	}
}

func TestMiddlewareUsesIncomingID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(Middleware())
	router.GET("/", func(c *gin.Context) {
		c.String(http.StatusOK, FromGin(c))
	})

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set(Header, "incoming-id")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if got := recorder.Header().Get(Header); got != "incoming-id" {
		t.Fatalf("response header = %q, want incoming-id", got)
	}
	if got := recorder.Body.String(); got != "incoming-id" {
		t.Fatalf("body = %q, want incoming-id", got)
	}
}

func TestMiddlewareGeneratesID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(Middleware())
	router.GET("/", func(c *gin.Context) {
		c.String(http.StatusOK, FromGin(c))
	})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))

	id := recorder.Header().Get(Header)
	if !Valid(id) {
		t.Fatalf("generated ID is invalid: %q", id)
	}
	if got := recorder.Body.String(); got != id {
		t.Fatalf("body = %q, want %q", got, id)
	}
}
