package config

import (
	"testing"
	"time"
)

func testLoader(values map[string]string) Loader {
	return NewLoader(func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	})
}

func TestLoaderStringAndRequired(t *testing.T) {
	loader := testLoader(map[string]string{"APP_ENV": "production", "EMPTY": ""})

	if got := loader.String("APP_ENV", "development"); got != "production" {
		t.Fatalf("String() = %q, want production", got)
	}
	if got := loader.String("MISSING", "development"); got != "development" {
		t.Fatalf("String() fallback = %q, want development", got)
	}

	if _, err := loader.Required("MISSING"); err == nil {
		t.Fatal("Required() should fail for a missing variable")
	}
	if _, err := loader.Required("EMPTY"); err == nil {
		t.Fatal("Required() should fail for an empty variable")
	}
	if got, err := loader.Required("APP_ENV"); err != nil || got != "production" {
		t.Fatalf("Required() = %q, %v", got, err)
	}
}

func TestLoaderPort(t *testing.T) {
	loader := testLoader(map[string]string{"PORT": "8081", "BAD_PORT": "abc", "OUT_OF_RANGE": "70000"})

	if got, err := loader.Port("PORT", 8080); err != nil || got != 8081 {
		t.Fatalf("Port() = %d, %v; want 8081", got, err)
	}
	if got, err := loader.Port("MISSING", 8080); err != nil || got != 8080 {
		t.Fatalf("Port() fallback = %d, %v; want 8080", got, err)
	}
	if _, err := loader.Port("BAD_PORT", 8080); err == nil {
		t.Fatal("Port() should fail for a non-integer value")
	}
	if _, err := loader.Port("OUT_OF_RANGE", 8080); err == nil {
		t.Fatal("Port() should fail for an out-of-range value")
	}
}

func TestLoaderDuration(t *testing.T) {
	loader := testLoader(map[string]string{"TIMEOUT": "15s", "BAD_TIMEOUT": "soon"})

	if got, err := loader.Duration("TIMEOUT", time.Second); err != nil || got != 15*time.Second {
		t.Fatalf("Duration() = %v, %v; want 15s", got, err)
	}
	if got, err := loader.Duration("MISSING", time.Second); err != nil || got != time.Second {
		t.Fatalf("Duration() fallback = %v, %v; want 1s", got, err)
	}
	if _, err := loader.Duration("BAD_TIMEOUT", time.Second); err == nil {
		t.Fatal("Duration() should fail for an invalid duration")
	}
}

func TestLoaderEnum(t *testing.T) {
	loader := testLoader(map[string]string{"APP_ENV": "production", "BAD_ENV": "local"})

	if got, err := loader.Enum("APP_ENV", "development", "development", "staging", "production"); err != nil || got != "production" {
		t.Fatalf("Enum() = %q, %v; want production", got, err)
	}
	if got, err := loader.Enum("MISSING", "development", "development", "staging", "production"); err != nil || got != "development" {
		t.Fatalf("Enum() fallback = %q, %v; want development", got, err)
	}
	if _, err := loader.Enum("BAD_ENV", "development", "development", "staging", "production"); err == nil {
		t.Fatal("Enum() should fail for an unsupported value")
	}
}
