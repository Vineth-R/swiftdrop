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
		case "APP_ENV":
			return "development", true
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

	if config.Port != 8086 {
		t.Fatalf("Port = %d, want 8086", config.Port)
	}
	if config.AccessSecret != "" {
		t.Fatalf("AccessSecret = %q, want empty", config.AccessSecret)
	}
	if config.CookieName != "sd_access" {
		t.Fatalf("CookieName = %q, want sd_access", config.CookieName)
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
	if config.ProcessedEventsRetention != 30*24*time.Hour {
		t.Fatalf("ProcessedEventsRetention = %v, want 720h", config.ProcessedEventsRetention)
	}
	if config.ProcessedEventsCleanupInterval != 24*time.Hour {
		t.Fatalf("ProcessedEventsCleanupInterval = %v, want 24h", config.ProcessedEventsCleanupInterval)
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
	if config.IdentityBaseURL != "http://identity:8081" {
		t.Fatalf("IdentityBaseURL = %q", config.IdentityBaseURL)
	}
	if config.IdentityTimeout != 3*time.Second {
		t.Fatalf("IdentityTimeout = %v, want 3s", config.IdentityTimeout)
	}
	if config.ParcelCreatedQueue != "notification.parcel.created.v1" || config.ParcelCreatedRoutingKey != "parcel.created.v1" {
		t.Fatalf("parcel created config = %q/%q", config.ParcelCreatedQueue, config.ParcelCreatedRoutingKey)
	}
	if config.ParcelStatusQueue != "notification.parcel.status_changed.v1" || config.ParcelStatusRoutingKey != "parcel.status_changed.v1" {
		t.Fatalf("parcel status config = %q/%q", config.ParcelStatusQueue, config.ParcelStatusRoutingKey)
	}
	if config.FrontendBaseURL != "http://localhost:8080" || config.SupportEmail != "" {
		t.Fatalf("email site config = %q/%q", config.FrontendBaseURL, config.SupportEmail)
	}
	if config.InternalToken != "" || config.InternalTokenProblem == "" {
		t.Fatalf("internal token config = %q/%q, want empty token and a problem", config.InternalToken, config.InternalTokenProblem)
	}
}

func TestLoadFromParsesHTTPConfig(t *testing.T) {
	loader := newLoader(map[string]string{
		"RABBITMQ_URL":      "amqp://swiftdrop:swiftdrop@rabbitmq:5672/",
		"PORT":              "18086",
		"JWT_ACCESS_SECRET": "shared-access-secret",
		"COOKIE_NAME":       "custom_access",
	})

	config, err := LoadFrom(loader)
	if err != nil {
		t.Fatalf("LoadFrom() returned error: %v", err)
	}
	if config.Port != 18086 || config.Addr() != ":18086" {
		t.Fatalf("Port = %d, Addr = %q", config.Port, config.Addr())
	}
	if config.AccessSecret != "shared-access-secret" || config.CookieName != "custom_access" {
		t.Fatalf("HTTP auth config = %q / %q", config.AccessSecret, config.CookieName)
	}
}

func TestLoadFromParsesProcessedEventRetentionConfig(t *testing.T) {
	loader := newLoader(map[string]string{
		"RABBITMQ_URL": "amqp://swiftdrop:swiftdrop@rabbitmq:5672/",
		"NOTIFICATION_PROCESSED_EVENTS_RETENTION":        "48h",
		"NOTIFICATION_PROCESSED_EVENTS_CLEANUP_INTERVAL": "6h",
	})

	config, err := LoadFrom(loader)
	if err != nil {
		t.Fatalf("LoadFrom() returned error: %v", err)
	}
	if config.ProcessedEventsRetention != 48*time.Hour {
		t.Fatalf("ProcessedEventsRetention = %v, want 48h", config.ProcessedEventsRetention)
	}
	if config.ProcessedEventsCleanupInterval != 6*time.Hour {
		t.Fatalf("ProcessedEventsCleanupInterval = %v, want 6h", config.ProcessedEventsCleanupInterval)
	}
}

func TestLoadFromRejectsInvalidProcessedEventRetentionConfig(t *testing.T) {
	tests := map[string]map[string]string{
		"zero retention":      {"NOTIFICATION_PROCESSED_EVENTS_RETENTION": "0s"},
		"negative retention":  {"NOTIFICATION_PROCESSED_EVENTS_RETENTION": "-1h"},
		"malformed retention": {"NOTIFICATION_PROCESSED_EVENTS_RETENTION": "not-a-duration"},
		"zero interval":       {"NOTIFICATION_PROCESSED_EVENTS_CLEANUP_INTERVAL": "0s"},
		"negative interval":   {"NOTIFICATION_PROCESSED_EVENTS_CLEANUP_INTERVAL": "-1h"},
		"malformed interval":  {"NOTIFICATION_PROCESSED_EVENTS_CLEANUP_INTERVAL": "not-a-duration"},
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

func TestLoadFromRejectsInvalidPort(t *testing.T) {
	loader := newLoader(map[string]string{
		"RABBITMQ_URL": "amqp://swiftdrop:swiftdrop@rabbitmq:5672/",
		"PORT":         "not-a-port",
	})

	if _, err := LoadFrom(loader); err == nil {
		t.Fatal("LoadFrom() should fail for an invalid PORT")
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
		case "APP_ENV":
			return "development", true
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
		case "APP_ENV":
			return "development", true
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
		case "APP_ENV":
			return "development", true
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

func TestLoadFromRequiresAppEnv(t *testing.T) {
	tests := map[string]map[string]string{
		"unset":      {},
		"empty":      {"APP_ENV": ""},
		"whitespace": {"APP_ENV": "   "},
		"unknown":    {"APP_ENV": "local"},
	}

	for name, values := range tests {
		t.Run(name, func(t *testing.T) {
			loader := sharedconfig.NewLoader(func(key string) (string, bool) {
				value, ok := values[key]
				return value, ok
			})
			if _, err := LoadFrom(loader); err == nil {
				t.Fatalf("LoadFrom() accepted APP_ENV=%q", values["APP_ENV"])
			}
		})
	}
}

func TestLoadFromExplicitAppEnv(t *testing.T) {
	for _, appEnv := range []string{"development", "staging", "production"} {
		t.Run(appEnv, func(t *testing.T) {
			loader := newLoader(map[string]string{
				"APP_ENV":      appEnv,
				"RABBITMQ_URL": "amqp://swiftdrop:swiftdrop@rabbitmq:5672/",
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

func TestLoadFromAccessSecret(t *testing.T) {
	weak := strings.Repeat("w", sharedconfig.MinAccessSecretBytes-1)
	valid := strings.Repeat("v", sharedconfig.MinAccessSecretBytes)
	placeholder := "change-me-access-secret-use-32-or-more-random-characters"

	tests := []struct {
		name    string
		appEnv  string
		secret  string
		want    string
		wantErr bool
	}{
		{name: "development weak", appEnv: "development", secret: weak, want: weak},
		{name: "development placeholder", appEnv: "development", secret: placeholder, want: placeholder},
		{name: "production weak", appEnv: "production", secret: weak, wantErr: true},
		{name: "production placeholder", appEnv: "production", secret: placeholder, wantErr: true},
		{name: "production uppercase placeholder", appEnv: "production", secret: "CHANGEME-" + strings.Repeat("x", 40), wantErr: true},
		{name: "production unknown short", appEnv: "production", secret: "unknown-secret", wantErr: true},
		{name: "production 32 byte", appEnv: "production", secret: valid, want: valid},
		{name: "production empty", appEnv: "production", secret: "", want: ""},
		{name: "staging weak", appEnv: "staging", secret: weak, wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			values := map[string]string{
				"APP_ENV":      test.appEnv,
				"RABBITMQ_URL": "amqp://swiftdrop:swiftdrop@rabbitmq:5672/",
			}
			if test.secret != "" {
				values["JWT_ACCESS_SECRET"] = test.secret
			}

			config, err := LoadFrom(newLoader(values))
			if test.wantErr {
				if err == nil {
					t.Fatal("LoadFrom() should have failed")
				}
				return
			}
			if err != nil {
				t.Fatalf("LoadFrom() returned error: %v", err)
			}
			if config.AccessSecret != test.want {
				t.Fatalf("AccessSecret = %q, want %q", config.AccessSecret, test.want)
			}
		})
	}
}

func TestLoadFromParsesIdentityAndParcelConfig(t *testing.T) {
	loader := newLoader(map[string]string{
		"RABBITMQ_URL":                            "amqp://swiftdrop:swiftdrop@rabbitmq:5672/",
		"APP_ENV":                                 "development",
		"INTERNAL_TOKEN":                          "change-me-internal-token",
		"IDENTITY_BASE_URL":                       "https://identity.example:8443/",
		"IDENTITY_TIMEOUT":                        "2s",
		"NOTIFICATION_PARCEL_CREATED_QUEUE":       "notification.parcel.created.test",
		"NOTIFICATION_PARCEL_CREATED_ROUTING_KEY": "parcel.created.v1",
		"NOTIFICATION_PARCEL_STATUS_QUEUE":        "notification.parcel.status_changed.test",
		"NOTIFICATION_PARCEL_STATUS_ROUTING_KEY":  "parcel.status_changed.v1",
		"FRONTEND_BASE_URL":                       "https://app.example.test/",
		"SUPPORT_EMAIL":                           "support@example.test",
	})

	config, err := LoadFrom(loader)
	if err != nil {
		t.Fatalf("LoadFrom() returned error: %v", err)
	}
	if config.IdentityBaseURL != "https://identity.example:8443" {
		t.Fatalf("IdentityBaseURL = %q", config.IdentityBaseURL)
	}
	if config.IdentityTimeout != 2*time.Second {
		t.Fatalf("IdentityTimeout = %v, want 2s", config.IdentityTimeout)
	}
	if config.InternalToken != "change-me-internal-token" || config.InternalTokenProblem != "" {
		t.Fatalf("internal token config = %q/%q", config.InternalToken, config.InternalTokenProblem)
	}
	if config.ParcelCreatedQueue != "notification.parcel.created.test" ||
		config.ParcelCreatedRoutingKey != "parcel.created.v1" ||
		config.ParcelStatusQueue != "notification.parcel.status_changed.test" ||
		config.ParcelStatusRoutingKey != "parcel.status_changed.v1" {
		t.Fatalf("parcel queue config = %q/%q, %q/%q",
			config.ParcelCreatedQueue, config.ParcelCreatedRoutingKey,
			config.ParcelStatusQueue, config.ParcelStatusRoutingKey)
	}
	if config.FrontendBaseURL != "https://app.example.test" || config.SupportEmail != "support@example.test" {
		t.Fatalf("email site config = %q/%q", config.FrontendBaseURL, config.SupportEmail)
	}
}

func TestLoadFromRejectsInvalidIdentityConfig(t *testing.T) {
	tests := []struct {
		name   string
		values map[string]string
	}{
		{name: "identity base URL", values: map[string]string{"IDENTITY_BASE_URL": "identity:8081"}},
		{name: "identity timeout", values: map[string]string{"IDENTITY_TIMEOUT": "0s"}},
		{name: "duplicate queue", values: map[string]string{"NOTIFICATION_PARCEL_CREATED_QUEUE": "notification.requested.v1"}},
		{name: "duplicate parcel queue", values: map[string]string{"NOTIFICATION_PARCEL_CREATED_QUEUE": "notification.parcel.status_changed.v1"}},
		{name: "duplicate routing key", values: map[string]string{"NOTIFICATION_PARCEL_CREATED_ROUTING_KEY": "parcel.status_changed.v1"}},
		{name: "frontend base URL", values: map[string]string{"FRONTEND_BASE_URL": "app.example.test"}},
		{name: "support email", values: map[string]string{"SUPPORT_EMAIL": "not-an-email"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			values := map[string]string{"RABBITMQ_URL": "amqp://swiftdrop:swiftdrop@rabbitmq:5672/"}
			for key, value := range test.values {
				values[key] = value
			}
			if _, err := LoadFrom(newLoader(values)); err == nil {
				t.Fatal("LoadFrom() error = nil, want a configuration error")
			}
		})
	}
}
