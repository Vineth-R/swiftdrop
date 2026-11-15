package config

import (
	"errors"
	"fmt"
	"strings"
	"time"

	sharedconfig "github.com/Vineth-R/swiftdrop/packages/go-common/config"
	"github.com/Vineth-R/swiftdrop/packages/webhookdelivery"
)

const (
	// defaultQueue is the webhook worker's own durable queue. A separate queue
	// per worker keeps retries isolated now that both the notification worker
	// and the webhook worker consume parcel.status_changed.v1.
	defaultQueue = "webhook.parcel.status_changed.v1"

	// defaultRoutingKey is the event the worker listens for: the parcel service
	// publishes it to the swiftdrop.events exchange whenever a parcel changes
	// status.
	defaultRoutingKey = "parcel.status_changed.v1"

	// legacyQueue and legacyRoutingKey are the retired webhook work item. It has
	// no producer. Deployments that still set either value keep their
	// configuration, but they receive no parcel status events.
	legacyQueue      = "webhook.delivery.requested.v1"
	legacyRoutingKey = "webhook.delivery.requested.v1"
)

type Config struct {
	AppEnv         string
	LogLevel       string
	Port           int
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
	// RetryDelays is an explicitly development-only schedule used by the
	// real-stack retry test. Empty keeps the API v1.1 production schedule.
	RetryDelays []time.Duration
	// TestWebhookOrigin is an explicitly development-only HTTP receiver origin.
	TestWebhookOrigin string
}

// UsesLegacyQueue reports whether the resolved configuration points at the
// retired webhook work item. Callers should warn at startup; the configured
// values are never rewritten.
func (c Config) UsesLegacyQueue() bool {
	return c.Queue == legacyQueue || c.RoutingKey == legacyRoutingKey
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

	port, err := loader.Port("PORT", 8088)
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

	queue := loader.String("WEBHOOK_QUEUE", defaultQueue)

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

	retryDelays, err := parseTestRetryDelays(loader.String("WEBHOOK_RETRY_DELAYS", ""), appEnv)
	if err != nil {
		return Config{}, err
	}
	testOrigin := strings.TrimSpace(loader.String("WEBHOOK_TEST_ORIGIN", ""))
	if testOrigin != "" {
		if appEnv != "development" {
			return Config{}, errors.New("WEBHOOK_TEST_ORIGIN is only allowed in development")
		}
		normalized, err := webhookdelivery.NormalizeTestOrigin(testOrigin)
		if err != nil {
			return Config{}, fmt.Errorf("WEBHOOK_TEST_ORIGIN: %w", err)
		}
		testOrigin = normalized
	}

	return Config{
		AppEnv:            appEnv,
		LogLevel:          logLevel,
		Port:              port,
		RabbitMQURL:       rabbitMQURL,
		Exchange:          loader.String("RABBITMQ_EXCHANGE", "swiftdrop.events"),
		Queue:             queue,
		RoutingKey:        loader.String("WEBHOOK_ROUTING_KEY", defaultRoutingKey),
		Prefetch:          prefetch,
		HandlerTimeout:    handlerTimeout,
		MaxRetries:        maxRetries,
		RetryDelay:        retryDelay,
		ReconnectMin:      reconnectMin,
		ReconnectMax:      reconnectMax,
		RetryDelays:       retryDelays,
		TestWebhookOrigin: testOrigin,
	}, nil
}

func parseTestRetryDelays(raw, appEnv string) ([]time.Duration, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	if appEnv != "development" {
		return nil, errors.New("WEBHOOK_RETRY_DELAYS is only allowed in development")
	}
	parts := strings.Split(raw, ",")
	if len(parts) != webhookdelivery.MaxAttempts-1 {
		return nil, fmt.Errorf("WEBHOOK_RETRY_DELAYS must contain %d comma-separated durations", webhookdelivery.MaxAttempts-1)
	}
	delays := make([]time.Duration, 0, len(parts))
	for _, part := range parts {
		delay, err := time.ParseDuration(strings.TrimSpace(part))
		if err != nil || delay < time.Second {
			return nil, errors.New("WEBHOOK_RETRY_DELAYS entries must be durations of at least 1s")
		}
		delays = append(delays, delay)
	}
	return delays, nil
}

// Addr returns the internal HTTP listen address for health and readiness.
func (c Config) Addr() string {
	return ":" + fmt.Sprint(c.Port)
}
