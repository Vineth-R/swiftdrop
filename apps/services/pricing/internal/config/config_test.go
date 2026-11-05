package config

import (
	"strings"
	"testing"

	sharedconfig "github.com/Vineth-R/swiftdrop/packages/go-common/config"
)

func TestLoadFromExplicitDevelopmentDefaults(t *testing.T) {
	loader := sharedconfig.NewLoader(func(key string) (string, bool) {
		if key == "APP_ENV" {
			return "development", true
		}
		return "", false
	})

	config, err := LoadFrom(loader)
	if err != nil {
		t.Fatalf("LoadFrom() returned error: %v", err)
	}

	if config.Port != 8083 {
		t.Fatalf("Port = %d, want 8083", config.Port)
	}
	if config.AppEnv != "development" {
		t.Fatalf("AppEnv = %q, want development", config.AppEnv)
	}
	if config.LogLevel != "info" {
		t.Fatalf("LogLevel = %q, want info", config.LogLevel)
	}
	if config.Addr() != ":8083" {
		t.Fatalf("Addr() = %q, want :8083", config.Addr())
	}
}

func TestLoadFromRequiresExplicitAppEnv(t *testing.T) {
	tests := []struct {
		name   string
		values map[string]string
	}{
		{name: "unset"},
		{name: "empty", values: map[string]string{"APP_ENV": ""}},
		{name: "whitespace", values: map[string]string{"APP_ENV": "   "}},
		{name: "unknown", values: map[string]string{"APP_ENV": "local"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			loader := sharedconfig.NewLoader(func(key string) (string, bool) {
				value, ok := test.values[key]
				return value, ok
			})
			if _, err := LoadFrom(loader); err == nil {
				t.Fatal("LoadFrom() should fail without an explicit valid APP_ENV")
			}
		})
	}
}

func TestLoadFromIsCaseInsensitiveAndTrims(t *testing.T) {
	loader := sharedconfig.NewLoader(func(key string) (string, bool) {
		values := map[string]string{
			"APP_ENV":        " PRODUCTION ",
			"LOG_LEVEL":      "WARNING",
			"PORT":           " 9092 ",
			"DATABASE_URL":   " postgres://example ",
			"INTERNAL_TOKEN": strings.Repeat("t", sharedconfig.MinInternalTokenLengthNonDevelopment),
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

func TestLoadFromInternalTokenValidation(t *testing.T) {
	validDevelopment := strings.Repeat("d", sharedconfig.MinInternalTokenLength)
	validNonDevelopment := strings.Repeat("n", sharedconfig.MinInternalTokenLengthNonDevelopment)
	placeholder := "change-me-internal-token"

	for _, test := range []struct {
		name       string
		appEnv     string
		envPresent bool
		value      string
		wantToken  string
		wantError  bool
	}{
		{name: "unset APP_ENV placeholder", envPresent: false, value: placeholder, wantError: true},
		{name: "empty APP_ENV placeholder", appEnv: "", envPresent: true, value: placeholder, wantError: true},
		{name: "whitespace APP_ENV placeholder", appEnv: "   ", envPresent: true, value: placeholder, wantError: true},
		{name: "unknown APP_ENV", appEnv: "local", envPresent: true, value: validNonDevelopment, wantError: true},
		{name: "explicit development unset", appEnv: "development", envPresent: true},
		{name: "explicit development too short", appEnv: "development", envPresent: true, value: strings.Repeat("d", sharedconfig.MinInternalTokenLength-1)},
		{name: "explicit development minimum", appEnv: "development", envPresent: true, value: validDevelopment, wantToken: validDevelopment},
		{name: "explicit development placeholder", appEnv: "development", envPresent: true, value: placeholder, wantToken: placeholder},
		{name: "explicit staging placeholder", appEnv: "staging", envPresent: true, value: placeholder, wantError: true},
		{name: "explicit staging valid", appEnv: "staging", envPresent: true, value: validNonDevelopment, wantToken: validNonDevelopment},
		{name: "explicit staging too short", appEnv: "staging", envPresent: true, value: validDevelopment, wantError: true},
		{name: "explicit production placeholder", appEnv: "production", envPresent: true, value: placeholder, wantError: true},
		{name: "explicit production valid", appEnv: "production", envPresent: true, value: validNonDevelopment, wantToken: validNonDevelopment},
		{name: "explicit production too short", appEnv: "production", envPresent: true, value: validDevelopment, wantError: true},
		{name: "explicit production unset", appEnv: "production", envPresent: true, wantError: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			loader := sharedconfig.NewLoader(func(key string) (string, bool) {
				switch key {
				case "APP_ENV":
					return test.appEnv, test.envPresent
				case "INTERNAL_TOKEN":
					return test.value, test.value != ""
				default:
					return "", false
				}
			})
			config, err := LoadFrom(loader)
			if test.wantError {
				if err == nil {
					t.Fatal("LoadFrom() should fail")
				}
				if test.value != "" && strings.Contains(err.Error(), test.value) {
					t.Fatal("the error text contains the token")
				}
				return
			}
			if err != nil {
				t.Fatalf("LoadFrom() returned error: %v", err)
			}
			if config.InternalToken != test.wantToken {
				t.Fatalf("InternalToken = %q, want %q", config.InternalToken, test.wantToken)
			}
		})
	}
}
