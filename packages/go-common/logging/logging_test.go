package logging

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func TestParseLevel(t *testing.T) {
	tests := map[string]slog.Level{
		"":        slog.LevelInfo,
		"debug":   slog.LevelDebug,
		"info":    slog.LevelInfo,
		"warn":    slog.LevelWarn,
		"warning": slog.LevelWarn,
		"ERROR":   slog.LevelError,
	}

	for input, expected := range tests {
		got, err := ParseLevel(input)
		if err != nil {
			t.Fatalf("ParseLevel(%q) returned error: %v", input, err)
		}
		if got != expected {
			t.Fatalf("ParseLevel(%q) = %v, want %v", input, got, expected)
		}
	}

	if _, err := ParseLevel("verbose"); err == nil {
		t.Fatal("ParseLevel() should fail for an unsupported level")
	}
}

func TestNewValidatesLevel(t *testing.T) {
	if _, err := New("identity", "production", "verbose"); err == nil {
		t.Fatal("New() should fail for an unsupported level")
	}

	logger, err := New("identity", "development", "info")
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}
	if logger == nil {
		t.Fatal("New() returned a nil logger")
	}
}

func TestNewAcceptsMixedCaseInputs(t *testing.T) {
	if _, err := New("identity", "DEVELOPMENT", "WARNING"); err != nil {
		t.Fatalf("New() returned error: %v", err)
	}
}

func TestNewBootstrapRedactsStartupFailures(t *testing.T) {
	var buffer bytes.Buffer
	logger := newBootstrap(&buffer, "identity")

	logger.Error("service stopped",
		"database_url", "postgres://user:s3cret@db:5432/app",
		"rabbitmq_url", "amqp://user:s3cret@rabbitmq:5672/",
		"dsn", "host=db password=s3cret",
		"connection_string", "Endpoint=db;Password=s3cret",
		"smtp_user", "ops@example.com",
		"error", "startup failed",
	)

	output := buffer.String()
	for _, secret := range []string{"s3cret", "postgres://", "amqp://", "host=db", "Endpoint=db", "ops@example.com"} {
		if strings.Contains(output, secret) {
			t.Fatalf("bootstrap log leaked %q: %s", secret, output)
		}
	}
	if !strings.Contains(output, `"service":"identity"`) {
		t.Fatalf("bootstrap log is missing the service field: %s", output)
	}
	if !strings.Contains(output, "startup failed") {
		t.Fatalf("bootstrap log dropped the safe error message: %s", output)
	}
}
