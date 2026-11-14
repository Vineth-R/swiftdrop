package httpx

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

type readinessPinger struct{ err error }

func (p readinessPinger) Ping(context.Context) error { return p.err }

func readinessRouter(deps ...ReadinessDependency) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/ready", ReadinessHandler(deps...))
	return router
}

func readinessResult(t *testing.T, router *gin.Engine) (int, readinessResponse) {
	t.Helper()
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/ready", nil))
	var body readinessResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode readiness: %v", err)
	}
	return recorder.Code, body
}

func TestReadinessHandlerReady(t *testing.T) {
	code, body := readinessResult(t, readinessRouter(
		DatabaseDependency(readinessPinger{}, time.Second),
		BooleanDependency("consumer", func() bool { return true }),
	))
	if code != http.StatusOK || body.Status != "ready" {
		t.Fatalf("code/status = %d/%q", code, body.Status)
	}
	if body.Checks["database"] != "ok" || body.Checks["consumer"] != "ok" {
		t.Fatalf("checks = %v", body.Checks)
	}
}

func TestReadinessHandlerDatabaseDown(t *testing.T) {
	code, body := readinessResult(t, readinessRouter(
		DatabaseDependency(readinessPinger{err: errors.New("down")}, time.Second),
	))
	if code != http.StatusServiceUnavailable || body.Checks["database"] != "down" {
		t.Fatalf("code/checks = %d/%v", code, body.Checks)
	}
}

func TestReadinessHandlerNilPingerAndNilDependencyFailClosed(t *testing.T) {
	code, body := readinessResult(t, readinessRouter(
		DatabaseDependency(nil, time.Second),
		ReadinessDependency{Name: "consumer"},
	))
	if code != http.StatusServiceUnavailable || body.Checks["database"] != "down" || body.Checks["consumer"] != "down" {
		t.Fatalf("code/checks = %d/%v", code, body.Checks)
	}
}
