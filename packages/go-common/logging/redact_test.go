package logging

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func TestRedactingHandlerMasksCredentialsAndPII(t *testing.T) {
	var buffer bytes.Buffer
	logger := slog.New(NewRedactingHandler(slog.NewJSONHandler(&buffer, nil)))

	logger.Info("login attempt",
		"password", "Str0ngPass!",
		"accessToken", "eyJhbGciOi-secret",
		"authorization", "Bearer abc123",
		"cookie", "sd_refresh=rt_9fA",
		"phone", "+94771234567",
		"pickup_address", "12 Temple Road, Nugegoda",
		"body", `{"password":"Str0ngPass!","refreshToken":"rt_9fA"}`,
		"tracking_number", "SDK7M2Q9XP4",
	)

	output := buffer.String()
	for _, secret := range []string{"Str0ngPass!", "eyJhbGciOi-secret", "Bearer abc123", "sd_refresh=rt_9fA", "+94771234567", "12 Temple Road"} {
		if strings.Contains(output, secret) {
			t.Fatalf("log output leaked %q: %s", secret, output)
		}
	}
	if !strings.Contains(output, "SDK7M2Q9XP4") {
		t.Fatalf("non-sensitive attribute was removed: %s", output)
	}
	if !strings.Contains(output, redacted) {
		t.Fatalf("expected redaction marker in %s", output)
	}
}

func TestRedactingHandlerMasksGroupsAndAttachedAttrs(t *testing.T) {
	var buffer bytes.Buffer
	base := slog.New(NewRedactingHandler(slog.NewJSONHandler(&buffer, nil)))
	logger := base.With("api_key", "sdk_live_secret").WithGroup("request")

	logger.Info("handled", "token", "rt_secret", "status", "ok")

	output := buffer.String()
	if strings.Contains(output, "sdk_live_secret") || strings.Contains(output, "rt_secret") {
		t.Fatalf("log output leaked a secret: %s", output)
	}
	if !strings.Contains(output, "status") {
		t.Fatalf("non-sensitive attribute was removed: %s", output)
	}
}

func TestBodyForLogNeverReturnsRequestBodies(t *testing.T) {
	body := []byte(`{"email":"nimal@example.com","password":"Str0ngPass!","refreshToken":"rt_9fA"}`)

	for _, path := range []string{"/auth/login", "/api/v1/auth/login", "/users/me/password", "/parcels"} {
		got := BodyForLog(path, body)
		if got != redacted {
			t.Fatalf("BodyForLog(%q) = %q, want %q", path, got, redacted)
		}
	}
}

func TestIsAuthPath(t *testing.T) {
	auth := []string{"/auth/login", "/api/v1/auth/refresh", "/users/me/password", "/auth/reset-password"}
	for _, path := range auth {
		if !IsAuthPath(path) {
			t.Fatalf("IsAuthPath(%q) = false, want true", path)
		}
	}
	if IsAuthPath("/parcels") {
		t.Fatal("IsAuthPath() should be false for a business path")
	}
}
