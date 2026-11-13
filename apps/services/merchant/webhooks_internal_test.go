package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	serviceconfig "github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/config"
)

const (
	testInternalToken = "internal-route-test-token-32-bytes!"
	testInternalPath  = "/internal/merchants/5f0b2c0e-6c53-4f6e-9a38-0f4b1f0e7a11/webhook"
)

func internalConfig(key, token string) serviceconfig.Config {
	return serviceconfig.Config{AppEnv: "development", WebhookSecretKey: key, InternalToken: token}
}

func callInternal(router *gin.Engine, method, path, token string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, nil)
	if token != "" {
		request.Header.Set("X-Internal-Token", token)
	}
	request.Header.Set("X-Request-Id", "req-1")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}

func TestRegisterInternalWebhooksAddsThreeRoutes(t *testing.T) {
	router := gin.New()
	if err := registerInternalWebhooks(router, nil, internalConfig(testWebhookKey, testInternalToken), quietLogger()); err != nil {
		t.Fatal(err)
	}
	if got := len(router.Routes()); got != 3 {
		t.Fatalf("routes = %d, want 3", got)
	}
}

func TestRegisterInternalWebhooksWithoutTokenRefusesEveryCall(t *testing.T) {
	router := gin.New()
	cfg := internalConfig(testWebhookKey, "")
	cfg.InternalTokenProblem = "INTERNAL_TOKEN is not set"
	if err := registerInternalWebhooks(router, nil, cfg, quietLogger()); err != nil {
		t.Fatalf("an unset token must not stop the service: %v", err)
	}
	for _, token := range []string{"", testInternalToken} {
		if recorder := callInternal(router, http.MethodGet, testInternalPath, token); recorder.Code != http.StatusUnauthorized {
			t.Fatalf("token %q: status = %d, want 401", token, recorder.Code)
		}
	}
}

func TestRegisterInternalWebhooksWithoutKeyAnswers503(t *testing.T) {
	router := gin.New()
	if err := registerInternalWebhooks(router, nil, internalConfig("", testInternalToken), quietLogger()); err != nil {
		t.Fatalf("an unset key must not stop the service: %v", err)
	}
	if recorder := callInternal(router, http.MethodGet, testInternalPath, testInternalToken); recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", recorder.Code)
	}
	if recorder := callInternal(router, http.MethodGet, testInternalPath, ""); recorder.Code != http.StatusUnauthorized {
		t.Fatalf("without the token: status = %d, want 401", recorder.Code)
	}
}

func TestRegisterInternalWebhooksWithoutRepositoryStaysClosed(t *testing.T) {
	router := gin.New()
	if err := registerInternalWebhooks(router, nil, internalConfig(testWebhookKey, testInternalToken), quietLogger()); err != nil {
		t.Fatal(err)
	}
	// A valid key and token but no database: still unavailable, never a panic.
	if recorder := callInternal(router, http.MethodGet, testInternalPath, testInternalToken); recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", recorder.Code)
	}
}

func TestRegisterInternalWebhooksRejectsInvalidKey(t *testing.T) {
	for _, key := range []string{"not-hex", "abcd", strings.Repeat("zz", 32)} {
		if err := registerInternalWebhooks(gin.New(), nil, internalConfig(key, testInternalToken), quietLogger()); err == nil {
			t.Errorf("key %q was accepted", key)
		}
	}
}
