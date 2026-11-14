package config

import (
	"reflect"
	"strings"
	"testing"
	"time"

	sharedconfig "github.com/Vineth-R/swiftdrop/packages/go-common/config"
)

const testBroker = "amqp://swiftdrop:swiftdrop@rabbitmq:5672/"

func loaderFor(values map[string]string) sharedconfig.Loader {
	return sharedconfig.NewLoader(func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	})
}

// valid returns the smallest set of values that loads, with overrides applied.
func valid(overrides map[string]string) map[string]string {
	values := map[string]string{
		"APP_ENV":      "development",
		"RABBITMQ_URL": testBroker,
	}
	for key, value := range overrides {
		values[key] = value
	}
	return values
}

func TestLoadFromDefaults(t *testing.T) {
	config, err := LoadFrom(loaderFor(valid(nil)))
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
	if config.CookieName != "sd_access" {
		t.Fatalf("CookieName = %q, want sd_access", config.CookieName)
	}
	if config.AccessSecret != "" {
		t.Fatalf("AccessSecret = %q, want empty", config.AccessSecret)
	}
	if config.Exchange != "swiftdrop.events" {
		t.Fatalf("Exchange = %q", config.Exchange)
	}
	if config.Queue != "realtime.parcel.status_changed.v1" {
		t.Fatalf("Queue = %q", config.Queue)
	}
	if config.RoutingKey != "parcel.status_changed.v1" {
		t.Fatalf("RoutingKey = %q", config.RoutingKey)
	}
	if config.AssignedQueue != "realtime.parcel.assigned.v1" {
		t.Fatalf("AssignedQueue = %q", config.AssignedQueue)
	}
	if config.AssignedRoutingKey != "parcel.assigned.v1" {
		t.Fatalf("AssignedRoutingKey = %q", config.AssignedRoutingKey)
	}
	if config.Prefetch != 10 || config.MaxRetries != 3 {
		t.Fatalf("Prefetch = %d, MaxRetries = %d", config.Prefetch, config.MaxRetries)
	}
	if config.HandlerTimeout != 30*time.Second || config.RetryDelay != 30*time.Second {
		t.Fatalf("HandlerTimeout = %s, RetryDelay = %s", config.HandlerTimeout, config.RetryDelay)
	}
	if config.ReconnectMin != time.Second || config.ReconnectMax != 30*time.Second {
		t.Fatalf("ReconnectMin = %s, ReconnectMax = %s", config.ReconnectMin, config.ReconnectMax)
	}
}

func TestLoadFromRequiresAppEnv(t *testing.T) {
	tests := []struct {
		name   string
		values map[string]string
	}{
		{name: "unset", values: map[string]string{"RABBITMQ_URL": testBroker}},
		{name: "empty", values: map[string]string{"APP_ENV": "", "RABBITMQ_URL": testBroker}},
		{name: "whitespace", values: map[string]string{"APP_ENV": "   ", "RABBITMQ_URL": testBroker}},
		{name: "unknown", values: map[string]string{"APP_ENV": "local", "RABBITMQ_URL": testBroker}},
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
			config, err := LoadFrom(loaderFor(valid(map[string]string{"APP_ENV": appEnv})))
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
	loader := loaderFor(valid(map[string]string{
		"APP_ENV":      " PRODUCTION ",
		"LOG_LEVEL":    "WARNING",
		"PORT":         " 9092 ",
		"DATABASE_URL": " postgres://example ",
	}))

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
			loader := loaderFor(valid(map[string]string{"PORT": port}))
			if _, err := LoadFrom(loader); err == nil {
				t.Fatalf("LoadFrom() should fail for PORT=%q", port)
			}
		})
	}
}

func TestLoadFromRejectsInvalidLogLevel(t *testing.T) {
	loader := loaderFor(valid(map[string]string{"LOG_LEVEL": "verbose"}))
	if _, err := LoadFrom(loader); err == nil {
		t.Fatal("LoadFrom() should fail for an unsupported LOG_LEVEL")
	}
}

func TestLoadFromRequiresBroker(t *testing.T) {
	if _, err := LoadFrom(loaderFor(map[string]string{"APP_ENV": "development"})); err == nil {
		t.Fatal("LoadFrom() should fail without RABBITMQ_URL")
	}
}

func TestLoadFromRejectsBadBrokerURLWithoutLeakingIt(t *testing.T) {
	const secretURL = "http://user:topsecret@rabbitmq:5672/"
	_, err := LoadFrom(loaderFor(valid(map[string]string{"RABBITMQ_URL": secretURL})))
	if err == nil {
		t.Fatal("LoadFrom() should reject a URL that is not amqp or amqps")
	}
	if strings.Contains(err.Error(), "topsecret") {
		t.Fatalf("error leaks the broker URL: %v", err)
	}
}

func TestLoadFromBrokerSettings(t *testing.T) {
	loader := loaderFor(valid(map[string]string{
		"RABBITMQ_EXCHANGE":             "custom.events",
		"REALTIME_QUEUE":                "realtime.custom",
		"REALTIME_ROUTING_KEY":          "parcel.custom.v1",
		"REALTIME_ASSIGNED_QUEUE":       "realtime.assigned.custom",
		"REALTIME_ASSIGNED_ROUTING_KEY": "parcel.assigned.custom.v1",
		"RABBITMQ_PREFETCH":             "5",
		"RABBITMQ_HANDLER_TIMEOUT":      "10s",
		"RABBITMQ_MAX_RETRIES":          "0",
		"RABBITMQ_RETRY_DELAY":          "2s",
		"RABBITMQ_RECONNECT_MIN":        "2s",
		"RABBITMQ_RECONNECT_MAX":        "20s",
	}))
	config, err := LoadFrom(loader)
	if err != nil {
		t.Fatalf("LoadFrom() returned error: %v", err)
	}
	if config.Exchange != "custom.events" || config.Queue != "realtime.custom" || config.RoutingKey != "parcel.custom.v1" {
		t.Fatalf("topology = %q / %q / %q", config.Exchange, config.Queue, config.RoutingKey)
	}
	if config.AssignedQueue != "realtime.assigned.custom" || config.AssignedRoutingKey != "parcel.assigned.custom.v1" {
		t.Fatalf("assigned topology = %q / %q", config.AssignedQueue, config.AssignedRoutingKey)
	}
	if config.Prefetch != 5 || config.MaxRetries != 0 || config.RetryDelay != 2*time.Second {
		t.Fatalf("prefetch = %d, retries = %d, delay = %s", config.Prefetch, config.MaxRetries, config.RetryDelay)
	}
}

func TestLoadFromBlankTopologyFallsBackToDefaults(t *testing.T) {
	config, err := LoadFrom(loaderFor(valid(map[string]string{
		"REALTIME_QUEUE":       "   ",
		"REALTIME_ROUTING_KEY": "",
	})))
	if err != nil {
		t.Fatalf("LoadFrom() returned error: %v", err)
	}
	if config.Queue != "realtime.parcel.status_changed.v1" || config.RoutingKey != "parcel.status_changed.v1" {
		t.Fatalf("Queue = %q, RoutingKey = %q", config.Queue, config.RoutingKey)
	}
}

func TestLoadFromRejectsInvalidBrokerSettings(t *testing.T) {
	tests := []struct {
		name      string
		overrides map[string]string
	}{
		{name: "zero prefetch", overrides: map[string]string{"RABBITMQ_PREFETCH": "0"}},
		{name: "negative retries", overrides: map[string]string{"RABBITMQ_MAX_RETRIES": "-1"}},
		{name: "retry delay too small", overrides: map[string]string{"RABBITMQ_RETRY_DELAY": "0s"}},
		{name: "reconnect bounds reversed", overrides: map[string]string{"RABBITMQ_RECONNECT_MIN": "10s", "RABBITMQ_RECONNECT_MAX": "5s"}},
		{name: "bad duration", overrides: map[string]string{"RABBITMQ_HANDLER_TIMEOUT": "soon"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := LoadFrom(loaderFor(valid(test.overrides))); err == nil {
				t.Fatal("LoadFrom() should fail")
			}
		})
	}
}

func TestLoadFromAccessSecret(t *testing.T) {
	strong := strings.Repeat("k", 40)

	config, err := LoadFrom(loaderFor(valid(map[string]string{
		"APP_ENV":           "production",
		"JWT_ACCESS_SECRET": strong,
		"COOKIE_NAME":       "session",
	})))
	if err != nil {
		t.Fatalf("LoadFrom() returned error: %v", err)
	}
	if config.AccessSecret != strong || config.CookieName != "session" {
		t.Fatalf("AccessSecret/CookieName = %q / %q", config.AccessSecret, config.CookieName)
	}

	for name, secret := range map[string]string{
		"short":       "too-short",
		"placeholder": "change-me-" + strings.Repeat("x", 40),
	} {
		t.Run("rejects "+name+" outside development", func(t *testing.T) {
			_, err := LoadFrom(loaderFor(valid(map[string]string{
				"APP_ENV":           "production",
				"JWT_ACCESS_SECRET": secret,
			})))
			if err == nil {
				t.Fatal("LoadFrom() should reject a weak secret outside development")
			}
		})
	}

	// An unset secret is allowed so anonymous tracking keeps working.
	if _, err := LoadFrom(loaderFor(valid(map[string]string{"APP_ENV": "production"}))); err != nil {
		t.Fatalf("an unset secret should load: %v", err)
	}
}

func TestLoadFromAllowedOrigins(t *testing.T) {
	t.Run("development allows the Next.js dev server and the gateway by default", func(t *testing.T) {
		config, err := LoadFrom(loaderFor(valid(nil)))
		if err != nil {
			t.Fatalf("LoadFrom() returned error: %v", err)
		}
		want := []string{"localhost:3000", "127.0.0.1:3000", "localhost:8080", "127.0.0.1:8080"}
		if !reflect.DeepEqual(config.AllowedOrigins, want) {
			t.Fatalf("AllowedOrigins = %v, want %v", config.AllowedOrigins, want)
		}
	})

	t.Run("other environments have no default, so list the public hosts", func(t *testing.T) {
		for _, appEnv := range []string{"staging", "production"} {
			config, err := LoadFrom(loaderFor(valid(map[string]string{"APP_ENV": appEnv})))
			if err != nil {
				t.Fatalf("LoadFrom() returned error: %v", err)
			}
			if len(config.AllowedOrigins) != 0 {
				t.Fatalf("%s AllowedOrigins = %v, want none", appEnv, config.AllowedOrigins)
			}
		}
	})

	t.Run("a configured list replaces the default and is trimmed", func(t *testing.T) {
		config, err := LoadFrom(loaderFor(valid(map[string]string{
			"REALTIME_ALLOWED_ORIGINS": " app.example.test , ,staging.example.test:8443",
		})))
		if err != nil {
			t.Fatalf("LoadFrom() returned error: %v", err)
		}
		want := []string{"app.example.test", "staging.example.test:8443"}
		if !reflect.DeepEqual(config.AllowedOrigins, want) {
			t.Fatalf("AllowedOrigins = %v, want %v", config.AllowedOrigins, want)
		}
	})

	t.Run("rejects URLs and the wildcard", func(t *testing.T) {
		for _, bad := range []string{"*", "https://app.example.test", "app.example.test/path", "two words"} {
			_, err := LoadFrom(loaderFor(valid(map[string]string{"REALTIME_ALLOWED_ORIGINS": bad})))
			if err == nil {
				t.Fatalf("LoadFrom() should reject REALTIME_ALLOWED_ORIGINS=%q", bad)
			}
		}
	})
}

func TestLoadFromIdentitySessionSettings(t *testing.T) {
	config, err := LoadFrom(loaderFor(valid(nil)))
	if err != nil {
		t.Fatalf("LoadFrom() returned error: %v", err)
	}
	if config.IdentityBaseURL != "http://identity:8081" {
		t.Fatalf("IdentityBaseURL = %q", config.IdentityBaseURL)
	}
	if config.IdentityTimeout != 3*time.Second {
		t.Fatalf("IdentityTimeout = %s", config.IdentityTimeout)
	}
	if config.SessionCheckInterval != time.Minute {
		t.Fatalf("SessionCheckInterval = %s", config.SessionCheckInterval)
	}

	config, err = LoadFrom(loaderFor(valid(map[string]string{
		"IDENTITY_BASE_URL":               "http://identity.internal:8081/",
		"IDENTITY_TIMEOUT":                "5s",
		"REALTIME_SESSION_CHECK_INTERVAL": "30s",
	})))
	if err != nil {
		t.Fatalf("LoadFrom() returned error: %v", err)
	}
	if config.IdentityBaseURL != "http://identity.internal:8081" || config.IdentityTimeout != 5*time.Second {
		t.Fatalf("identity settings = %q/%s", config.IdentityBaseURL, config.IdentityTimeout)
	}
	if config.SessionCheckInterval != 30*time.Second {
		t.Fatalf("SessionCheckInterval = %s, want 30s", config.SessionCheckInterval)
	}
}

func TestLoadFromRejectsNegativeSessionCheckInterval(t *testing.T) {
	if _, err := LoadFrom(loaderFor(valid(map[string]string{"REALTIME_SESSION_CHECK_INTERVAL": "-1s"}))); err == nil {
		t.Fatal("LoadFrom() accepted a negative session check interval")
	}
}
