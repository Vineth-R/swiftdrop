package servicehttp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Vineth-R/swiftdrop/packages/go-common/messaging"
)

type fakePinger struct {
	err error
}

func (p fakePinger) Ping(context.Context) error { return p.err }

func readyRecorder(t *testing.T, pinger interface {
	Ping(context.Context) error
}, dependencies ...Dependency) (*httptest.ResponseRecorder, readinessResponse) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/ready", ReadyHandler(pinger, 50*time.Millisecond, dependencies...))

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/ready", nil))

	var body readinessResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode readiness body: %v", err)
	}
	return recorder, body
}

func TestReadyHandlerAllDependenciesReady(t *testing.T) {
	recorder, body := readyRecorder(t,
		fakePinger{},
		Dependency{Name: "consumer.status_changed", Ready: func() bool { return true }},
		Dependency{Name: "consumer.assigned", Ready: func() bool { return true }},
	)

	if recorder.Code != http.StatusOK || body.Status != "ready" {
		t.Fatalf("status = %d body = %+v, want 200 ready", recorder.Code, body)
	}
	for _, name := range []string{"database", "consumer.status_changed", "consumer.assigned"} {
		if body.Checks[name] != "ok" {
			t.Fatalf("check %q = %q, want ok", name, body.Checks[name])
		}
	}
}

func TestReadyHandlerDatabaseDown(t *testing.T) {
	recorder, body := readyRecorder(t,
		fakePinger{err: errors.New("connection refused")},
		Dependency{Name: "consumer.status_changed", Ready: func() bool { return true }},
	)

	if recorder.Code != http.StatusServiceUnavailable || body.Status != "unavailable" {
		t.Fatalf("status = %d body = %+v, want 503 unavailable", recorder.Code, body)
	}
	if body.Checks["database"] != "down" {
		t.Fatalf("database check = %q, want down", body.Checks["database"])
	}
	if body.Checks["consumer.status_changed"] != "ok" {
		t.Fatalf("consumer check = %q, want ok", body.Checks["consumer.status_changed"])
	}
}

func TestReadyHandlerConsumerDown(t *testing.T) {
	recorder, body := readyRecorder(t,
		fakePinger{},
		Dependency{Name: "consumer.status_changed", Ready: func() bool { return true }},
		Dependency{Name: "consumer.assigned", Ready: func() bool { return false }},
	)

	if recorder.Code != http.StatusServiceUnavailable || body.Status != "unavailable" {
		t.Fatalf("status = %d body = %+v, want 503 unavailable", recorder.Code, body)
	}
	if body.Checks["consumer.assigned"] != "down" {
		t.Fatalf("assigned consumer check = %q, want down", body.Checks["consumer.assigned"])
	}
}

func TestReadyHandlerNilPingerAndNilCheckFailClosed(t *testing.T) {
	recorder, body := readyRecorder(t, nil, Dependency{Name: "consumer.assigned"})

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", recorder.Code)
	}
	if body.Checks["database"] != "down" || body.Checks["consumer.assigned"] != "down" {
		t.Fatalf("checks = %v, want database and consumer down", body.Checks)
	}
}

func TestReadyHandlerRecoversWhenConsumerReconnects(t *testing.T) {
	health := messaging.NewHealth()
	health.MarkReconnecting(errors.New("connection lost"))
	recorder, body := readyRecorder(t,
		fakePinger{},
		Dependency{Name: "consumer.status_changed", Ready: health.Ready},
	)
	if recorder.Code != http.StatusServiceUnavailable || body.Checks["consumer.status_changed"] != "down" {
		t.Fatalf("reconnecting consumer reported ready: %d %+v", recorder.Code, body)
	}

	health.MarkReady()
	recorder, body = readyRecorder(t,
		fakePinger{},
		Dependency{Name: "consumer.status_changed", Ready: health.Ready},
	)
	if recorder.Code != http.StatusOK || body.Checks["consumer.status_changed"] != "ok" {
		t.Fatalf("ready consumer not reported ready: %d %+v", recorder.Code, body)
	}
}
