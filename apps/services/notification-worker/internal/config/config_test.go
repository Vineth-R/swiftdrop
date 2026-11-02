package config

import (
	"strings"
	"testing"
	"time"

	sharedconfig "github.com/Vineth-R/swiftdrop/packages/go-common/config"
)

func newLoader(values map[string]string) sharedconfig.Loader {
	return sharedconfig.NewLoader(func(key string) (string, bool) {
		if value, ok := values[key]; ok {
			return value, true
		}
		switch key {
		case "DATABASE_URL":
			return "postgres://swiftdrop:swiftdrop@postgres:5432/swiftdrop?sslmode=disable", true
		case "SMTP_HOST":
			return "mailpit", true
		case "SMTP_PORT":
			return "1025", true
		default:
			return "", false
		}
	})
}

func TestLoadFrom(t *testing.T) {
	loader := newLoader(map[string]string{
		"APP_ENV":      "production",
		"LOG_LEVEL":    "warn",
		"RABBITMQ_URL": "amqp://swiftdrop:swiftdrop@rabbitmq:5672/",
	})

	config, err := LoadFrom(loader)
	if err != nil {
		t.Fatalf("LoadFrom() returned error: %v", err)
	}

	if config.Queue != "notification.requested.v1" {
		t.Fatalf("Queue = %q", config.Queue)
	}
	if config.RoutingKey != config.Queue {
		t.Fatalf("RoutingKey = %q, want %q", config.RoutingKey, config.Queue)
	}
	if config.Prefetch != 10 {
		t.Fatalf("Prefetch = %d, want 10", config.Prefetch)
	}
	if config.HandlerTimeout != 30*time.Second {
		t.Fatalf("HandlerTimeout = %v, want 30s", config.HandlerTimeout)
	}
	if config.MaxRetries != 3 {
		t.Fatalf("MaxRetries = %d, want 3", config.MaxRetries)
	}
	if config.RetryDelay != 30*time.Second {
		t.Fatalf("RetryDelay = %v, want 30s", config.RetryDelay)
	}
	if config.DatabaseURL != "postgres://swiftdrop:swiftdrop@postgres:5432/swiftdrop?sslmode=disable" {
		t.Fatalf("DatabaseURL = %q", config.DatabaseURL)
	}
	if config.SMTP.Host != "mailpit" || config.SMTP.Port != 1025 {
		t.Fatalf("SMTP = %+v, want mailpit:1025", config.SMTP)
	}
	if err := config.SMTP.Validate(); err != nil {
		t.Fatalf("SMTP config does not validate: %v", err)
	}
}

func TestLoadFromParsesRetryConfig(t *testing.T) {
	loader := newLoader(map[string]string{
		"RABBITMQ_URL":         "amqp://swiftdrop:swiftdrop@rabbitmq:5672/",
		"RABBITMQ_MAX_RETRIES": "5",
		"RABBITMQ_RETRY_DELAY": "2s",
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
			if _, err := LoadFrom(newLoader(values)); err == nil {
				t.Fatal("LoadFrom() should have failed")
			}
		})
	}
}

func TestLoadFromRequiresDatabaseURL(t *testing.T) {
	loader := sharedconfig.NewLoader(func(key string) (string, bool) {
		switch key {
		case "RABBITMQ_URL":
			return "amqp://swiftdrop:swiftdrop@rabbitmq:5672/", true
		case "SMTP_HOST":
			return "mailpit", true
		case "SMTP_PORT":
			return "1025", true
		default:
			return "", false
		}
	})

	if _, err := LoadFrom(loader); err == nil {
		t.Fatal("LoadFrom() should fail without DATABASE_URL")
	}
}

func TestLoadFromRequiresRabbitMQURL(t *testing.T) {
	loader := sharedconfig.NewLoader(func(key string) (string, bool) {
		switch key {
		case "DATABASE_URL":
			return "postgres://swiftdrop:swiftdrop@postgres:5432/swiftdrop?sslmode=disable", true
		case "SMTP_HOST":
			return "mailpit", true
		case "SMTP_PORT":
			return "1025", true
		default:
			return "", false
		}
	})

	if _, err := LoadFrom(loader); err == nil {
		t.Fatal("LoadFrom() should fail without RABBITMQ_URL")
	}
}

func TestLoadFromRequiresSMTPHost(t *testing.T) {
	loader := sharedconfig.NewLoader(func(key string) (string, bool) {
		switch key {
		case "DATABASE_URL":
			return "postgres://swiftdrop:swiftdrop@postgres:5432/swiftdrop?sslmode=disable", true
		case "RABBITMQ_URL":
			return "amqp://swiftdrop:swiftdrop@rabbitmq:5672/", true
		default:
			return "", false
		}
	})

	if _, err := LoadFrom(loader); err == nil {
		t.Fatal("LoadFrom() should fail without SMTP_HOST")
	}
}

func TestLoadFromRejectsInvalidSMTPConfig(t *testing.T) {
	tests := map[string]map[string]string{
		"invalid port":       {"SMTP_PORT": "not-a-port"},
		"port out of range":  {"SMTP_PORT": "70000"},
		"user without pass":  {"SMTP_USER": "user@example.com"},
		"pass without user":  {"SMTP_PASSWORD": "secret"},
		"sender with a name": {"SMTP_FROM": "Support <no-reply@swiftdrop.local>"},
		"from name newline":  {"SMTP_FROM_NAME": "Evil\r\nBcc: victim@example.com"},
	}

	for name, values := range tests {
		t.Run(name, func(t *testing.T) {
			values["RABBITMQ_URL"] = "amqp://swiftdrop:swiftdrop@rabbitmq:5672/"
			values["SMTP_HOST"] = "mailpit"
			if _, err := LoadFrom(newLoader(values)); err == nil {
				t.Fatal("LoadFrom() should have failed")
			}
		})
	}
}

func TestLoadFromRejectsInvalidRabbitMQURL(t *testing.T) {
	loader := newLoader(map[string]string{
		"RABBITMQ_URL": "http://rabbitmq:5672",
	})

	if _, err := LoadFrom(loader); err == nil {
		t.Fatal("LoadFrom() should fail for a non-AMQP URL")
	}
}

func TestLoadFromDoesNotLeakRabbitMQCredentials(t *testing.T) {
	loader := newLoader(map[string]string{
		"RABBITMQ_URL": "amqp://swiftdrop:s3cr3t-password@rabbitmq:5672/%zz",
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

func TestLoadFromDoesNotLeakSMTPPassword(t *testing.T) {
	loader := newLoader(map[string]string{
		"RABBITMQ_URL":  "amqp://swiftdrop:swiftdrop@rabbitmq:5672/",
		"SMTP_HOST":     "smtp.example.com",
		"SMTP_USER":     "mailer@example.com",
		"SMTP_PASSWORD": "s3cr3t-smtp-password",
		"SMTP_FROM":     "not-an-address",
	})

	_, err := LoadFrom(loader)
	if err == nil {
		t.Fatal("LoadFrom() should fail for an invalid SMTP configuration")
	}
	if strings.Contains(err.Error(), "s3cr3t-smtp-password") {
		t.Fatalf("error leaked the SMTP password: %q", err.Error())
	}
}

func TestLoadFromIsCaseInsensitiveAndTrims(t *testing.T) {
	loader := newLoader(map[string]string{
		"APP_ENV":      " PRODUCTION ",
		"LOG_LEVEL":    "WARNING",
		"RABBITMQ_URL": " amqp://swiftdrop:swiftdrop@rabbitmq:5672/ ",
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
