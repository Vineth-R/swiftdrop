package config

import (
	"strings"
	"testing"
	"time"

	sharedconfig "github.com/Vineth-R/swiftdrop/packages/go-common/config"
)

func TestLoadFrom(t *testing.T) {
	loader := sharedconfig.NewLoader(func(key string) (string, bool) {
		values := map[string]string{
			"APP_ENV":      "production",
			"LOG_LEVEL":    "warn",
			"RABBITMQ_URL": "amqp://swiftdrop:swiftdrop@rabbitmq:5672/",
		}
		value, ok := values[key]
		return value, ok
	})

	config, err := LoadFrom(loader)
	if err != nil {
		t.Fatalf("LoadFrom() returned error: %v", err)
	}

	if config.Queue != "webhook.parcel.status_changed.v1" {
		t.Fatalf("Queue = %q", config.Queue)
	}
	if config.RoutingKey != "parcel.status_changed.v1" {
		t.Fatalf("RoutingKey = %q, want parcel.status_changed.v1", config.RoutingKey)
	}
	if config.MaxRetries != 3 {
		t.Fatalf("MaxRetries = %d, want 3", config.MaxRetries)
	}
	if config.RetryDelay != 30*time.Second {
		t.Fatalf("RetryDelay = %v, want 30s", config.RetryDelay)
	}
}

func TestLoadFromRejectsMissingOrInvalidAppEnv(t *testing.T) {
	tests := map[string]map[string]string{
		"unset":      {},
		"empty":      {"APP_ENV": ""},
		"whitespace": {"APP_ENV": "   "},
		"unknown":    {"APP_ENV": "local"},
	}

	for name, values := range tests {
		t.Run(name, func(t *testing.T) {
			values["RABBITMQ_URL"] = "amqp://swiftdrop:swiftdrop@rabbitmq:5672/"
			loader := sharedconfig.NewLoader(func(key string) (string, bool) {
				value, ok := values[key]
				return value, ok
			})

			_, err := LoadFrom(loader)
			if err == nil {
				t.Fatal("LoadFrom() should have failed")
			}
			if !strings.Contains(err.Error(), "APP_ENV") {
				t.Fatalf("error should reference APP_ENV, got %q", err.Error())
			}
		})
	}
}

func TestLoadFromAcceptsSupportedAppEnv(t *testing.T) {
	for _, appEnv := range []string{"development", "staging", "production"} {
		t.Run(appEnv, func(t *testing.T) {
			loader := sharedconfig.NewLoader(func(key string) (string, bool) {
				values := map[string]string{
					"APP_ENV":      appEnv,
					"RABBITMQ_URL": "amqp://swiftdrop:swiftdrop@rabbitmq:5672/",
				}
				value, ok := values[key]
				return value, ok
			})

			config, err := LoadFrom(loader)
			if err != nil {
				t.Fatalf("LoadFrom() returned error: %v", err)
			}
			if config.AppEnv != appEnv {
				t.Fatalf("AppEnv = %q, want %q", config.AppEnv, appEnv)
			}
		})
	}
}

func TestLoadFromParsesRetryConfig(t *testing.T) {
	loader := sharedconfig.NewLoader(func(key string) (string, bool) {
		values := map[string]string{
			"APP_ENV":              "development",
			"RABBITMQ_URL":         "amqp://swiftdrop:swiftdrop@rabbitmq:5672/",
			"RABBITMQ_MAX_RETRIES": "5",
			"RABBITMQ_RETRY_DELAY": "2s",
		}
		value, ok := values[key]
		return value, ok
	})

	config, err := LoadFrom(loader)
	if err != nil {
		t.Fatalf("LoadFrom() returned error: %v", err)
	}

	if config.MaxRetries != 5 {
		t.Fatalf("MaxRetries = %d, want 5", config.MaxRetries)
	}
	if config.RetryDelay != 2*time.Second {
		t.Fatalf("RetryDelay = %v, want 2s", config.RetryDelay)
	}
}

func TestLoadFromRejectsInvalidRetryConfig(t *testing.T) {
	tests := map[string]map[string]string{
		"negative max retries":  {"RABBITMQ_MAX_RETRIES": "-1"},
		"zero retry delay":      {"RABBITMQ_RETRY_DELAY": "0s"},
		"sub-millisecond delay": {"RABBITMQ_RETRY_DELAY": "1ns"},
	}

	for name, values := range tests {
		t.Run(name, func(t *testing.T) {
			values["APP_ENV"] = "development"
			values["RABBITMQ_URL"] = "amqp://swiftdrop:swiftdrop@rabbitmq:5672/"
			loader := sharedconfig.NewLoader(func(key string) (string, bool) {
				value, ok := values[key]
				return value, ok
			})

			if _, err := LoadFrom(loader); err == nil {
				t.Fatal("LoadFrom() should have failed")
			}
		})
	}
}

func TestLoadFromRejectsInvalidRabbitMQURL(t *testing.T) {
	loader := sharedconfig.NewLoader(func(key string) (string, bool) {
		switch key {
		case "APP_ENV":
			return "development", true
		case "RABBITMQ_URL":
			return "http://rabbitmq:5672", true
		}
		return "", false
	})

	if _, err := LoadFrom(loader); err == nil {
		t.Fatal("LoadFrom() should fail for a non-AMQP URL")
	}
}

func TestLoadFromDoesNotLeakRabbitMQCredentials(t *testing.T) {
	loader := sharedconfig.NewLoader(func(key string) (string, bool) {
		switch key {
		case "APP_ENV":
			return "development", true
		case "RABBITMQ_URL":
			return "amqp://swiftdrop:s3cr3t-password@rabbitmq:5672/%zz", true
		}
		return "", false
	})

	_, err := LoadFrom(loader)
	if err == nil {
		t.Fatal("LoadFrom() should fail for a malformed URL")
	}
	if strings.Contains(err.Error(), "s3cr3t-password") {
		t.Fatalf("error leaked the password: %q", err.Error())
	}
	if strings.Contains(err.Error(), "swiftdrop") {
		t.Fatalf("error leaked the username: %q", err.Error())
	}
}

func TestLoadFromIsCaseInsensitiveAndTrims(t *testing.T) {
	loader := sharedconfig.NewLoader(func(key string) (string, bool) {
		values := map[string]string{
			"APP_ENV":      " PRODUCTION ",
			"LOG_LEVEL":    "WARNING",
			"RABBITMQ_URL": " amqp://swiftdrop:swiftdrop@rabbitmq:5672/ ",
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
}

func TestUsesLegacyQueue(t *testing.T) {
	tests := map[string]struct {
		values map[string]string
		want   bool
	}{
		"defaults":           {},
		"new queue":          {values: map[string]string{"WEBHOOK_QUEUE": "webhook.parcel.status_changed.v1"}},
		"legacy queue":       {values: map[string]string{"WEBHOOK_QUEUE": "webhook.delivery.requested.v1"}, want: true},
		"legacy routing key": {values: map[string]string{"WEBHOOK_ROUTING_KEY": "webhook.delivery.requested.v1"}, want: true},
		"both legacy": {
			values: map[string]string{
				"WEBHOOK_QUEUE":       "webhook.delivery.requested.v1",
				"WEBHOOK_ROUTING_KEY": "webhook.delivery.requested.v1",
			},
			want: true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			values := map[string]string{
				"APP_ENV":      "development",
				"RABBITMQ_URL": "amqp://swiftdrop:swiftdrop@rabbitmq:5672/",
			}
			for key, value := range test.values {
				values[key] = value
			}

			loader := sharedconfig.NewLoader(func(key string) (string, bool) {
				value, ok := values[key]
				return value, ok
			})

			config, err := LoadFrom(loader)
			if err != nil {
				t.Fatalf("LoadFrom() returned error: %v", err)
			}
			if got := config.UsesLegacyQueue(); got != test.want {
				t.Fatalf("UsesLegacyQueue() = %v, want %v", got, test.want)
			}

			// Explicit configuration is reported, never rewritten.
			if value, ok := test.values["WEBHOOK_QUEUE"]; ok && config.Queue != value {
				t.Fatalf("Queue = %q, want %q", config.Queue, value)
			}
			if value, ok := test.values["WEBHOOK_ROUTING_KEY"]; ok && config.RoutingKey != value {
				t.Fatalf("RoutingKey = %q, want %q", config.RoutingKey, value)
			}
		})
	}
}
