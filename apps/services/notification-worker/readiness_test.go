package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/Vineth-R/swiftdrop/packages/go-common/httpx"
	"github.com/Vineth-R/swiftdrop/packages/go-common/messaging"
)

type readyPinger struct{}

func (readyPinger) Ping(context.Context) error { return nil }

func TestNotificationReadinessDoesNotDependOnCleanup(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := httpx.NewRouter(slog.New(slog.NewTextHandler(io.Discard, nil)))
	legacy := messaging.NewHealth()
	created := messaging.NewHealth()
	status := messaging.NewHealth()
	legacy.MarkReady()
	created.MarkReady()
	status.MarkReady()
	router.GET("/ready", notificationReadinessHandler(readyPinger{}, legacy, created, status))

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/ready", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("ready status = %d, want 200: %s", recorder.Code, recorder.Body.String())
	}
	body := recorder.Body.String()
	for _, expected := range []string{
		`"status":"ready"`,
		`"database":"ok"`,
		`"consumer.notification_requested":"ok"`,
		`"consumer.parcel_created":"ok"`,
		`"consumer.parcel_status_changed":"ok"`,
	} {
		if !strings.Contains(body, expected) {
			t.Fatalf("ready response missing %s: %s", expected, body)
		}
	}
	if strings.Contains(body, "cleanup") {
		t.Fatalf("cleanup must not be a readiness dependency: %s", body)
	}

	created.MarkReconnecting(errors.New("broker unavailable"))
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/ready", nil))
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("ready status after consumer loss = %d, want 503", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), `"consumer.parcel_created":"down"`) {
		t.Fatalf("ready response does not report the consumer: %s", recorder.Body.String())
	}
}
