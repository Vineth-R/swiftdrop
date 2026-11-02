package config

import (
	"errors"
	"fmt"
	"time"

	sharedconfig "github.com/Vineth-R/swiftdrop/packages/go-common/config"
	"github.com/Vineth-R/swiftdrop/packages/go-common/mailer"
)

type Config struct {
	AppEnv         string
	LogLevel       string
	SMTP           mailer.Config
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
	appEnv, err := loader.EnumFold("APP_ENV", "development", "development", "staging", "production")
	if err != nil {
		return Config{}, err
	}

	logLevel, err := loader.EnumFold("LOG_LEVEL", "info", "debug", "info", "warn", "warning", "error")
	if err != nil {
		return Config{}, err
	}

	smtp, err := mailer.LoadConfig(loader)
	if err != nil {
		return Config{}, fmt.Errorf("SMTP configuration: %w", err)
	}
	if err := smtp.Validate(); err != nil {
		return Config{}, fmt.Errorf("SMTP configuration: %w", err)
	}

	rabbitMQURL, err := loader.Required("RABBITMQ_URL")
	if err != nil {
		return Config{}, err
	}
	if err := sharedconfig.ValidateAMQPURL(rabbitMQURL); err != nil {
		return Config{}, fmt.Errorf("RABBITMQ_URL: %w", err)
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
		SMTP:           smtp,
		RabbitMQURL:    rabbitMQURL,
		Exchange:       loader.String("RABBITMQ_EXCHANGE", "swiftdrop.events"),
		Queue:          queue,
		RoutingKey:     loader.String("NOTIFICATION_ROUTING_KEY", queue),
		Prefetch:       prefetch,
		HandlerTimeout: handlerTimeout,
		MaxRetries:     maxRetries,
		RetryDelay:     retryDelay,
		ReconnectMin:   reconnectMin,
		ReconnectMax:   reconnectMax,
	}, nil
}
