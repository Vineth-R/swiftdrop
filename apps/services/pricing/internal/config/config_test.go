package config

import (
	"strings"
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
		name      string
		appEnv    string
		value     string
		wantToken string
		wantError bool
	}{
		{name: "development unset", appEnv: "development"},
		{name: "development too short", appEnv: "development", value: strings.Repeat("d", sharedconfig.MinInternalTokenLength-1)},
		{name: "development minimum", appEnv: "development", value: validDevelopment, wantToken: validDevelopment},
		{name: "development placeholder", appEnv: "development", value: placeholder, wantToken: placeholder},
		{name: "production valid", appEnv: "production", value: validNonDevelopment, wantToken: validNonDevelopment},
		{name: "production placeholder", appEnv: "production", value: placeholder, wantError: true},
		{name: "production too short", appEnv: "production", value: validDevelopment, wantError: true},
		{name: "production unset", appEnv: "production", wantError: true},
		{name: "staging too short", appEnv: "staging", value: validDevelopment, wantError: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			loader := sharedconfig.NewLoader(func(key string) (string, bool) {
				switch key {
				case "APP_ENV":
					return test.appEnv, true
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
