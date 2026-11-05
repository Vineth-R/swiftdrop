package config

import (
	"testing"

	sharedconfig "github.com/Vineth-R/swiftdrop/packages/go-common/config"
)

func TestLoadFromDefaults(t *testing.T) {
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

	if config.Port != 8084 {
		t.Fatalf("Port = %d, want 8084", config.Port)
	}
	if config.AppEnv != "development" {
		t.Fatalf("AppEnv = %q, want development", config.AppEnv)
	}
	if config.LogLevel != "info" {
		t.Fatalf("LogLevel = %q, want info", config.LogLevel)
	}
	if config.Addr() != ":8084" {
		t.Fatalf("Addr() = %q, want :8084", config.Addr())
	}
}

func TestLoadFromAppEnv(t *testing.T) {
	tests := []struct {
		name    string
		values  map[string]string
		want    string
		wantErr bool
	}{
		{name: "unset", wantErr: true},
		{name: "empty", values: map[string]string{"APP_ENV": ""}, wantErr: true},
		{name: "whitespace", values: map[string]string{"APP_ENV": "   "}, wantErr: true},
		{name: "unknown", values: map[string]string{"APP_ENV": "local"}, wantErr: true},
		{name: "development", values: map[string]string{"APP_ENV": "development"}, want: "development"},
		{name: "staging", values: map[string]string{"APP_ENV": "staging"}, want: "staging"},
		{name: "production", values: map[string]string{"APP_ENV": "production"}, want: "production"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			loader := sharedconfig.NewLoader(func(key string) (string, bool) {
				value, ok := test.values[key]
				return value, ok
			})

			config, err := LoadFrom(loader)
			if test.wantErr {
				if err == nil {
					t.Fatalf("LoadFrom() accepted APP_ENV=%q", test.values["APP_ENV"])
				}
				return
			}
			if err != nil {
				t.Fatalf("LoadFrom() returned error: %v", err)
			}
			if config.AppEnv != test.want {
				t.Fatalf("AppEnv = %q, want %q", config.AppEnv, test.want)
			}
		})
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

func TestLoadFromRejectsInvalidPortAndLogLevel(t *testing.T) {
	tests := []struct {
		name   string
		values map[string]string
	}{
		{name: "port not an integer", values: map[string]string{"PORT": "abc"}},
		{name: "port below range", values: map[string]string{"PORT": "0"}},
		{name: "port above range", values: map[string]string{"PORT": "70000"}},
		{name: "unknown log level", values: map[string]string{"LOG_LEVEL": "trace"}},
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

			if _, err := LoadFrom(loader); err == nil {
				t.Fatalf("LoadFrom() should fail for values %v", test.values)
			}
		})
	}
}
