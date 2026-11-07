package config

import (
	"errors"
	"fmt"
	"time"

	sharedconfig "github.com/Vineth-R/swiftdrop/packages/go-common/config"
)

// defaultRoutingKey is the event the worker listens for: the parcel service
// publishes it to the swiftdrop.events exchange whenever a parcel changes status.
const defaultRoutingKey = "parcel.status_changed.v1"

type Config struct {
	AppEnv         string
	LogLevel       string
	RabbitMQURL    string
	Exchange       string
	Queue          string
	RoutingKey     string
	Prefetch       int
	HandlerTimeout time.Duration
	MaxRetries     int
	RetryDelay     time.Duration
	ReconnectMin   time.Duration
	ReconnectMax   time.Duration
}

func Load() (Config, error) {
	return LoadFrom(sharedconfig.Load())
}

func LoadFrom(loader sharedconfig.Loader) (Config, error) {
	appEnv, err := sharedconfig.LoadAppEnv(loader)
	if err != nil {
		return Config{}, err
	}

	logLevel, err := loader.EnumFold("LOG_LEVEL", "info", "debug", "info", "warn", "warning", "error")
	if err != nil {
		return Config{}, err
	}

	rabbitMQURL, err := loader.Required("RABBITMQ_URL")
	if err != nil {
		return Config{}, err
	}
	if err := sharedconfig.ValidateAMQPURL(rabbitMQURL); err != nil {
		return Config{}, fmt.Errorf("RABBITMQ_URL: %w", err)
	}

	queue := loader.String("WEBHOOK_QUEUE", "webhook.delivery.requested.v1")

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

	maxRetries, err := loader.Int("RABBITMQ_MAX_RETRIES", 3)
	if err != nil {
		return Config{}, err
	}
	if maxRetries < 0 {
		return Config{}, errors.New("RABBITMQ_MAX_RETRIES must not be negative")
	}

	retryDelay, err := loader.Duration("RABBITMQ_RETRY_DELAY", 30*time.Second)
	if err != nil {
		return Config{}, err
	}
	if retryDelay < time.Millisecond {
		return Config{}, errors.New("RABBITMQ_RETRY_DELAY must be at least 1ms")
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
		RoutingKey:     loader.String("WEBHOOK_ROUTING_KEY", defaultRoutingKey),
		Prefetch:       prefetch,
		HandlerTimeout: handlerTimeout,
		MaxRetries:     maxRetries,
		RetryDelay:     retryDelay,
		ReconnectMin:   reconnectMin,
		ReconnectMax:   reconnectMax,
	}, nil
}
