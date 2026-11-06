package config

import (
	"testing"

	sharedconfig "github.com/Vineth-R/swiftdrop/packages/go-common/config"
)

func loaderFor(values map[string]string) sharedconfig.Loader {
	return sharedconfig.NewLoader(func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	})
}

func TestLoadFromDefaults(t *testing.T) {
	loader := loaderFor(map[string]string{"APP_ENV": "development"})

	config, err := LoadFrom(loader)
	if err != nil {
		t.Fatalf("LoadFrom() returned error: %v", err)
	}

	if config.Port != 8085 {
		t.Fatalf("Port = %d, want 8085", config.Port)
	}
	if config.AppEnv != "development" {
		t.Fatalf("AppEnv = %q, want development", config.AppEnv)
	}
	if config.LogLevel != "info" {
		t.Fatalf("LogLevel = %q, want info", config.LogLevel)
	}
	if config.Addr() != ":8085" {
		t.Fatalf("Addr() = %q, want :8085", config.Addr())
	}
}

func TestLoadFromRequiresAppEnv(t *testing.T) {
	tests := []struct {
		name   string
		values map[string]string
	}{
		{name: "unset", values: map[string]string{}},
		{name: "empty", values: map[string]string{"APP_ENV": ""}},
		{name: "whitespace", values: map[string]string{"APP_ENV": "   "}},
		{name: "unknown", values: map[string]string{"APP_ENV": "local"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := LoadFrom(loaderFor(test.values)); err == nil {
				t.Fatal("LoadFrom() should fail without a valid APP_ENV")
			}
		})
	}
}

func TestLoadFromAcceptsSupportedAppEnv(t *testing.T) {
	tests := []string{"development", "staging", "production"}
	for _, appEnv := range tests {
		t.Run(appEnv, func(t *testing.T) {
			config, err := LoadFrom(loaderFor(map[string]string{"APP_ENV": appEnv}))
			if err != nil {
				t.Fatalf("LoadFrom() returned error: %v", err)
			}
			if config.AppEnv != appEnv {
				t.Fatalf("AppEnv = %q, want %q", config.AppEnv, appEnv)
			}
		})
	}
}

func TestLoadFromIsCaseInsensitiveAndTrims(t *testing.T) {
	loader := loaderFor(map[string]string{
		"APP_ENV":      " PRODUCTION ",
		"LOG_LEVEL":    "WARNING",
		"PORT":         " 9092 ",
		"DATABASE_URL": " postgres://example ",
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

func TestLoadFromRejectsInvalidPort(t *testing.T) {
	tests := []string{"not-a-number", "0", "65536"}
	for _, port := range tests {
		t.Run(port, func(t *testing.T) {
			loader := loaderFor(map[string]string{"APP_ENV": "development", "PORT": port})
			if _, err := LoadFrom(loader); err == nil {
				t.Fatalf("LoadFrom() should fail for PORT=%q", port)
			}
		})
	}
}

func TestLoadFromRejectsInvalidLogLevel(t *testing.T) {
	loader := loaderFor(map[string]string{"APP_ENV": "development", "LOG_LEVEL": "verbose"})
	if _, err := LoadFrom(loader); err == nil {
		t.Fatal("LoadFrom() should fail for an unsupported LOG_LEVEL")
	}
}
