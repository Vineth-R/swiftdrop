package config

import (
	"reflect"
	"testing"
	"time"
)

func TestLoaderCIDRs(t *testing.T) {
	for _, test := range []struct {
		name, raw string
		want      []string
		invalid   bool
	}{
		{name: "unset"},
		{name: "trim and mask", raw: " 10.1.2.3/8 , 2001:db8::1/32 ", want: []string{"10.0.0.0/8", "2001:db8::/32"}},
		{name: "invalid address", raw: "not-a-network", invalid: true},
		{name: "missing prefix", raw: "127.0.0.1", invalid: true},
		{name: "empty entry", raw: "127.0.0.0/8,", invalid: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := testLoader(map[string]string{"PROXIES": test.raw}).CIDRs("PROXIES")
			if (err != nil) != test.invalid {
				t.Fatalf("error %v", err)
			}
			if !test.invalid && !reflect.DeepEqual(got, test.want) {
				t.Fatalf("got %v want %v", got, test.want)
			}
		})
	}
}

func testLoader(values map[string]string) Loader {
	return NewLoader(func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	})
}

func TestLoaderStringAndRequired(t *testing.T) {
	loader := testLoader(map[string]string{
		"APP_ENV":    "  production  ",
		"EMPTY":      "",
		"WHITESPACE": "   ",
	})

	if got := loader.String("APP_ENV", "development"); got != "production" {
		t.Fatalf("String() = %q, want production", got)
	}
	if got := loader.String("MISSING", "development"); got != "development" {
		t.Fatalf("String() fallback = %q, want development", got)
	}
	if got := loader.String("WHITESPACE", "development"); got != "development" {
		t.Fatalf("String() whitespace fallback = %q, want development", got)
	}

	if _, err := loader.Required("MISSING"); err == nil {
		t.Fatal("Required() should fail for a missing variable")
	}
	if _, err := loader.Required("EMPTY"); err == nil {
		t.Fatal("Required() should fail for an empty variable")
	}
	if _, err := loader.Required("WHITESPACE"); err == nil {
		t.Fatal("Required() should fail for a whitespace-only variable")
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

func TestLoaderEnumFold(t *testing.T) {
	loader := testLoader(map[string]string{
		"APP_ENV":   "  PRODUCTION ",
		"LOG_LEVEL": "WARNING",
		"BAD_ENV":   "local",
	})

	if got, err := loader.EnumFold("APP_ENV", "development", "development", "staging", "production"); err != nil || got != "production" {
		t.Fatalf("EnumFold() = %q, %v; want production", got, err)
	}
	if got, err := loader.EnumFold("LOG_LEVEL", "info", "debug", "info", "warn", "warning", "error"); err != nil || got != "warning" {
		t.Fatalf("EnumFold() = %q, %v; want warning", got, err)
	}
	if got, err := loader.EnumFold("MISSING", "development", "development", "staging", "production"); err != nil || got != "development" {
		t.Fatalf("EnumFold() fallback = %q, %v; want development", got, err)
	}
	if _, err := loader.EnumFold("BAD_ENV", "development", "development", "staging", "production"); err == nil {
		t.Fatal("EnumFold() should fail for an unsupported value")
	}
}

func TestLoaderTrimsNumericAndDurationValues(t *testing.T) {
	loader := testLoader(map[string]string{
		"PORT":    " 8080 ",
		"RETRIES": " 3 ",
		"TIMEOUT": " 15s ",
	})

	if got, err := loader.Port("PORT", 80); err != nil || got != 8080 {
		t.Fatalf("Port() = %d, %v; want 8080", got, err)
	}
	if got, err := loader.Int("RETRIES", 1); err != nil || got != 3 {
		t.Fatalf("Int() = %d, %v; want 3", got, err)
	}
	if got, err := loader.Duration("TIMEOUT", time.Second); err != nil || got != 15*time.Second {
		t.Fatalf("Duration() = %v, %v; want 15s", got, err)
	}
}
