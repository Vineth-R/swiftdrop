package config

import (
	"testing"

	sharedconfig "github.com/Vineth-R/swiftdrop/packages/go-common/config"
)

func TestLoadFrom(t *testing.T) {
	loader := sharedconfig.NewLoader(func(key string) (string, bool) {
		values := map[string]string{
			"APP_ENV":      "production",
			"LOG_LEVEL":    "debug",
			"PORT":         "9091",
			"DATABASE_URL": "postgres://example",
		}
		value, ok := values[key]
		return value, ok
	})

	config, err := LoadFrom(loader)
	if err != nil {
		t.Fatalf("LoadFrom() returned error: %v", err)
	}

	if config.Port != 9091 {
		t.Fatalf("Port = %d, want 9091", config.Port)
	}
	if config.AppEnv != "production" {
		t.Fatalf("AppEnv = %q, want production", config.AppEnv)
	}
	if config.LogLevel != "debug" {
		t.Fatalf("LogLevel = %q, want debug", config.LogLevel)
	}
	if config.Addr() != ":9091" {
		t.Fatalf("Addr() = %q, want :9091", config.Addr())
	}
}

func TestLoadFromGoogleOAuthConfig(t *testing.T) {
	loader := sharedconfig.NewLoader(func(key string) (string, bool) {
		values := map[string]string{
			"APP_ENV":              "development",
			"GOOGLE_CLIENT_ID":     "client-id-example",
			"GOOGLE_CLIENT_SECRET": "test-only-secret",
			"GOOGLE_REDIRECT_URI":  "http://localhost:8080/api/v1/auth/google/callback",
		}
		value, ok := values[key]
		return value, ok
	})

	config, err := LoadFrom(loader)
	if err != nil {
		t.Fatalf("LoadFrom() returned error: %v", err)
	}

	if config.GoogleClientID != "client-id-example" {
		t.Fatal("GoogleClientID was not loaded")
	}
	if config.GoogleClientSecret != "test-only-secret" {
		t.Fatal("GoogleClientSecret was not loaded")
	}
	if config.GoogleRedirectURI != "http://localhost:8080/api/v1/auth/google/callback" {
		t.Fatal("GoogleRedirectURI was not loaded")
	}
}

func TestLoadFromGoogleOAuthConfigIsOptional(t *testing.T) {
	tests := []struct {
		name   string
		values map[string]string
	}{
		{
			name: "missing",
		},
		{
			name: "blank",
			values: map[string]string{
				"GOOGLE_CLIENT_ID":     " ",
				"GOOGLE_CLIENT_SECRET": "\t",
				"GOOGLE_REDIRECT_URI":  "  ",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			loader := sharedconfig.NewLoader(func(key string) (string, bool) {
				if key == "APP_ENV" {
					return "development", true
				}
				value, ok := test.values[key]
				return value, ok
			})

			config, err := LoadFrom(loader)
			if err != nil {
				t.Fatalf("LoadFrom() returned error: %v", err)
			}

			if config.GoogleClientID != "" || config.GoogleClientSecret != "" || config.GoogleRedirectURI != "" {
				t.Fatal("Google OAuth settings should be empty when missing or blank")
			}
		})
	}
}

func TestLoadFromRequiresExplicitAppEnv(t *testing.T) {
	tests := []struct {
		name       string
		values     map[string]string
		wantAppEnv string
		wantError  bool
	}{
		{name: "unset", wantError: true},
		{name: "empty", values: map[string]string{"APP_ENV": ""}, wantError: true},
		{name: "whitespace", values: map[string]string{"APP_ENV": "   "}, wantError: true},
		{name: "unknown", values: map[string]string{"APP_ENV": "local"}, wantError: true},
		{name: "explicit development", values: map[string]string{"APP_ENV": "development"}, wantAppEnv: "development"},
		{name: "explicit staging", values: map[string]string{"APP_ENV": "staging"}, wantAppEnv: "staging"},
		{name: "explicit production", values: map[string]string{"APP_ENV": "production"}, wantAppEnv: "production"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			loader := sharedconfig.NewLoader(func(key string) (string, bool) {
				value, ok := test.values[key]
				return value, ok
			})

			config, err := LoadFrom(loader)
			if test.wantError {
				if err == nil {
					t.Fatal("LoadFrom() should fail without an explicit valid APP_ENV")
				}
				return
			}
			if err != nil {
				t.Fatalf("LoadFrom() returned error: %v", err)
			}
			if config.AppEnv != test.wantAppEnv {
				t.Fatalf("AppEnv = %q, want %q", config.AppEnv, test.wantAppEnv)
			}
		})
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

func TestLoadFromIsCaseInsensitiveAndTrims(t *testing.T) {
	loader := sharedconfig.NewLoader(func(key string) (string, bool) {
		values := map[string]string{
			"APP_ENV":      " PRODUCTION ",
			"LOG_LEVEL":    "WARNING",
			"PORT":         " 9092 ",
			"DATABASE_URL": " postgres://example ",
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
}
