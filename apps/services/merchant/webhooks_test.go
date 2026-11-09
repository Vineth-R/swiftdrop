package main

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	serviceconfig "github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/config"
)

func quietLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

const (
	testWebhookKey = "1f1f1f1f1f1f1f1f1f1f1f1f1f1f1f1f1f1f1f1f1f1f1f1f1f1f1f1f1f1f1f1f"
	testAccessKey  = "webhook-route-test-access-secret-32-bytes"
)

func webhookConfig(key, accessSecret string) serviceconfig.Config {
	return serviceconfig.Config{
		AppEnv:           "development",
		WebhookSecretKey: key,
		AccessSecret:     accessSecret,
		CookieName:       "sd_access",
	}
}

func TestRegisterWebhooksWithoutKeyKeepsRoutesFailClosed(t *testing.T) {
	router := gin.New()
	if err := registerWebhooks(router, nil, webhookConfig("", testAccessKey), quietLogger()); err != nil {
		t.Fatalf("an unset key must not stop the service: %v", err)
	}
	if got := len(router.Routes()); got != 4 {
		t.Fatalf("routes = %d, want 4", got)
	}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/merchants/me/webhook", nil)
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", recorder.Code)
	}
}

func TestRegisterWebhooksWithoutAccessSecretKeepsRoutesFailClosed(t *testing.T) {
	router := gin.New()
	if err := registerWebhooks(router, nil, webhookConfig(testWebhookKey, ""), quietLogger()); err != nil {
		t.Fatalf("an unset access secret must not stop the service: %v", err)
	}
	if got := len(router.Routes()); got != 4 {
		t.Fatalf("routes = %d, want 4", got)
	}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/merchants/me/webhook", nil)
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", recorder.Code)
	}
}

func TestRegisterWebhooksRejectsInvalidKey(t *testing.T) {
	for _, key := range []string{"not-hex", "abcd", strings.Repeat("zz", 32)} {
		if err := registerWebhooks(gin.New(), nil, webhookConfig(key, testAccessKey), quietLogger()); err == nil {
			t.Errorf("key %q was accepted", key)
		}
	}
}

func TestRegisterWebhooksWithValidKeyAddsFourRoutes(t *testing.T) {
	router := gin.New()
	if err := registerWebhooks(router, nil, webhookConfig(testWebhookKey, testAccessKey), quietLogger()); err != nil {
		t.Fatal(err)
	}
	if got := len(router.Routes()); got != 4 {
		t.Fatalf("routes = %d, want 4", got)
	}
	// With no repository the routes stay fail closed even though the key and
	// access secret are valid. The authenticated path is covered by the handler
	// tests, which run against a populated store.
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/merchants/me/webhook", nil)
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", recorder.Code)
	}
}
