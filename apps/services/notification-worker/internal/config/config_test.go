package config

import (
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
}

func TestLoadFromRequiresRabbitMQURL(t *testing.T) {
	loader := sharedconfig.NewLoader(func(string) (string, bool) {
		return "", false
	})

	if _, err := LoadFrom(loader); err == nil {
		t.Fatal("LoadFrom() should fail without RABBITMQ_URL")
	}
}
