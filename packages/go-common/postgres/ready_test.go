package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

type fakePinger struct {
	err error
}

func (f fakePinger) Ping(_ context.Context) error {
	return f.err
}

func runReady(t *testing.T, pinger Pinger) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.GET("/ready", ReadyHandler(pinger, 50*time.Millisecond))

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/ready", nil))
	return recorder
}

func TestReadyHandlerReady(t *testing.T) {
	recorder := runReady(t, fakePinger{})

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}

	var body readinessResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if body.Status != "ready" || body.Checks["database"] != "ok" {
		t.Fatalf("unexpected body: %+v", body)
	}
}

func TestReadyHandlerDatabaseDown(t *testing.T) {
	recorder := runReady(t, fakePinger{err: errors.New("connection refused")})

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusServiceUnavailable)
	}

	var body readinessResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if body.Status != "unavailable" || body.Checks["database"] != "down" {
		t.Fatalf("unexpected body: %+v", body)
	}
}

func TestReadyHandlerUnconfigured(t *testing.T) {
	recorder := runReady(t, nil)

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusServiceUnavailable)
	}
}

func TestHealthHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/health", HealthHandler())

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/health", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if recorder.Body.String() != `{"status":"ok"}` {
		t.Fatalf("body = %s", recorder.Body.String())
	}
}
