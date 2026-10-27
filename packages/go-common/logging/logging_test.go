package logging

import (
	"log/slog"
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
