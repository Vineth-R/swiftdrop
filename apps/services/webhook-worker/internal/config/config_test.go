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

	if config.Queue != "webhook.delivery.requested.v1" {
		t.Fatalf("Queue = %q", config.Queue)
	}
	if config.RoutingKey != config.Queue {
		t.Fatalf("RoutingKey = %q, want %q", config.RoutingKey, config.Queue)
	}
	if config.MaxRetries != 3 {
		t.Fatalf("MaxRetries = %d, want 3", config.MaxRetries)
	}
	if config.RetryDelay != 30*time.Second {
		t.Fatalf("RetryDelay = %v, want 30s", config.RetryDelay)
	}
}

func TestLoadFromParsesRetryConfig(t *testing.T) {
	loader := sharedconfig.NewLoader(func(key string) (string, bool) {
		values := map[string]string{
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
		if key == "RABBITMQ_URL" {
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
		if key == "RABBITMQ_URL" {
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
