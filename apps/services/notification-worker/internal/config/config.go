package config

import (
	"errors"
	"fmt"
	"net/url"
	"time"

	sharedconfig "github.com/Vineth-R/swiftdrop/packages/go-common/config"
)

type Config struct {
	AppEnv         string
	LogLevel       string
	RabbitMQURL    string
	Exchange       string
	Queue          string
	RoutingKey     string
	Prefetch       int
	HandlerTimeout time.Duration
	ReconnectMin   time.Duration
	ReconnectMax   time.Duration
}

func Load() (Config, error) {
	return LoadFrom(sharedconfig.Load())
}

func LoadFrom(loader sharedconfig.Loader) (Config, error) {
	appEnv, err := loader.Enum("APP_ENV", "development", "development", "staging", "production")
	if err != nil {
		return Config{}, err
	}

	logLevel, err := loader.Enum("LOG_LEVEL", "info", "debug", "info", "warn", "error")
	if err != nil {
		return Config{}, err
	}

	rabbitMQURL, err := loader.Required("RABBITMQ_URL")
	if err != nil {
		return Config{}, err
	}

	parsedURL, err := url.Parse(rabbitMQURL)
	if err != nil {
		return Config{}, fmt.Errorf("RABBITMQ_URL is invalid: %w", err)
	}
	if parsedURL.Scheme != "amqp" && parsedURL.Scheme != "amqps" {
		return Config{}, errors.New("RABBITMQ_URL must use the amqp or amqps scheme")
	}
	if parsedURL.Host == "" {
		return Config{}, errors.New("RABBITMQ_URL must include a host")
	}

	queue := loader.String("NOTIFICATION_QUEUE", "notification.requested.v1")

	prefetch, err := loader.Int("RABBITMQ_PREFETCH", 10)
	if err != nil {
		return Config{}, err
	}
	if prefetch < 1 {
		return Config{}, errors.New("RABBITMQ_PREFETCH must be at least 1")
	}

	handlerTimeout, err := loader.Duration("RABBITMQ_HANDLER_TIMEOUT", 30*time.Second)
	if err != nil {
		return Config{}, err
	}

	reconnectMin, err := loader.Duration("RABBITMQ_RECONNECT_MIN", time.Second)
	if err != nil {
		return Config{}, err
	}

	reconnectMax, err := loader.Duration("RABBITMQ_RECONNECT_MAX", 30*time.Second)
	if err != nil {
		return Config{}, err
	}
	if reconnectMax < reconnectMin {
		return Config{}, errors.New("RABBITMQ_RECONNECT_MAX must be greater than or equal to RABBITMQ_RECONNECT_MIN")
	}

	return Config{
		AppEnv:         appEnv,
		LogLevel:       logLevel,
		RabbitMQURL:    rabbitMQURL,
		Exchange:       loader.String("RABBITMQ_EXCHANGE", "swiftdrop.events"),
		Queue:          queue,
		RoutingKey:     loader.String("NOTIFICATION_ROUTING_KEY", queue),
		Prefetch:       prefetch,
		HandlerTimeout: handlerTimeout,
		ReconnectMin:   reconnectMin,
		ReconnectMax:   reconnectMax,
	}, nil
}
