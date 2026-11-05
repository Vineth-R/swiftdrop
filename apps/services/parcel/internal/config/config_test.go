package config

import (
	"testing"

	sharedconfig "github.com/Vineth-R/swiftdrop/packages/go-common/config"
)

func TestLoadFromDefaults(t *testing.T) {
	loader := sharedconfig.NewLoader(func(string) (string, bool) {
		return "", false
	})

	config, err := LoadFrom(loader)
	if err != nil {
		t.Fatalf("LoadFrom() returned error: %v", err)
	}

	if config.Port != 8082 {
		t.Fatalf("Port = %d, want 8082", config.Port)
	}
	if config.AppEnv != "development" {
		t.Fatalf("AppEnv = %q, want development", config.AppEnv)
	}
	if config.LogLevel != "info" {
		t.Fatalf("LogLevel = %q, want info", config.LogLevel)
	}
	if config.Addr() != ":8082" {
		t.Fatalf("Addr() = %q, want :8082", config.Addr())
	}
	if config.AccessSecret != "" {
		t.Fatalf("AccessSecret = %q, want empty", config.AccessSecret)
	}
	if config.CookieName != "sd_access" {
		t.Fatalf("CookieName = %q, want sd_access", config.CookieName)
	}
}

func TestLoadFromIsCaseInsensitiveAndTrims(t *testing.T) {
	loader := sharedconfig.NewLoader(func(key string) (string, bool) {
		values := map[string]string{
			"APP_ENV":           " PRODUCTION ",
			"LOG_LEVEL":         "WARNING",
			"PORT":              " 9092 ",
			"DATABASE_URL":      " postgres://example ",
			"JWT_ACCESS_SECRET": " report-secret ",
			"COOKIE_NAME":       " sd_ops ",
		}
		value, ok := values[key]
		return value, ok
	})

	config, err := LoadFrom(loader)
	if err != nil {
		t.Fatalf("LoadFrom() returned error: %v", err)
	}

	if config.AppEnv != "production" {
		t.Fatalf("AppEnv = %q, want production", config.AppEnv)
	}
	if config.LogLevel != "warning" {
		t.Fatalf("LogLevel = %q, want warning", config.LogLevel)
	}
	if config.Port != 9092 {
		t.Fatalf("Port = %d, want 9092", config.Port)
	}
	if config.DatabaseURL != "postgres://example" {
		t.Fatalf("DatabaseURL = %q", config.DatabaseURL)
	}
	if config.AccessSecret != "report-secret" {
		t.Fatalf("AccessSecret = %q, want report-secret", config.AccessSecret)
	}
	if config.CookieName != "sd_ops" {
		t.Fatalf("CookieName = %q, want sd_ops", config.CookieName)
	}
}

func TestLoadFromRejectsInvalidConfig(t *testing.T) {
	loader := sharedconfig.NewLoader(func(key string) (string, bool) {
		if key == "APP_ENV" {
			return "local", true
		}
		return "", false
	})

	if _, err := LoadFrom(loader); err == nil {
		t.Fatal("LoadFrom() should fail for an unsupported APP_ENV")
	}
}
