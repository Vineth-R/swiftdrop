package config

import (
	"testing"

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
