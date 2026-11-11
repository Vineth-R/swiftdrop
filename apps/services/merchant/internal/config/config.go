package config

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	sharedconfig "github.com/Vineth-R/swiftdrop/packages/go-common/config"
)

type Config struct {
	Port             int
	AppEnv           string
	LogLevel         string
	DatabaseURL      string
	AccessSecret     string
	CookieName       string
	WebhookSecretKey string

	// InternalToken guards the /internal endpoints. Empty keeps them closed:
	// every call is refused. InternalTokenProblem says why it is empty.
	InternalToken        string
	InternalTokenProblem string

	FrontendBaseURL string
	SupportEmail    string
	IdentityBaseURL string
	IdentityTimeout time.Duration
	PricingBaseURL  string
	PricingTimeout  time.Duration
	ParcelBaseURL   string
	ParcelTimeout   time.Duration

	// RabbitMQURL enables the merchant.sync.queue consumer. It may be empty
	// only in development; outside development a missing URL stops startup so a
	// production-like deployment cannot silently skip status synchronization.
	RabbitMQURL            string
	RabbitMQExchange       string
	MerchantSyncQueue      string
	MerchantSyncRoutingKey string
	RabbitMQPrefetch       int
	RabbitMQHandlerTimeout time.Duration
	RabbitMQMaxRetries     int
	RabbitMQRetryDelay     time.Duration
	RabbitMQReconnectMin   time.Duration
	RabbitMQReconnectMax   time.Duration
}

// SyncEnabled reports whether the merchant status consumer should run.
func (c Config) SyncEnabled() bool { return strings.TrimSpace(c.RabbitMQURL) != "" }

// ValidateRuntime enforces deployment requirements that are not needed by the
// config parser's focused tests. Local development may run without a broker;
// staging and production must not silently skip merchant status synchronization.
func (c Config) ValidateRuntime() error {
	if !c.SyncEnabled() && c.AppEnv != "development" {
		return errors.New("RABBITMQ_URL is required outside development for merchant status synchronization")
	}
	return nil
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

	port, err := loader.Port("PORT", 8084)
	if err != nil {
		return Config{}, err
	}

	accessSecret, err := sharedconfig.LoadAccessSecret(loader, appEnv, false)
	if err != nil {
		return Config{}, err
	}

	webhookSecretKey, err := LoadWebhookSecretKey(loader)
	if err != nil {
		return Config{}, err
	}

	// Same rule as the other services: in development a missing or short token
	// closes the internal endpoints; elsewhere a token that is set but weak
	// (placeholder, under 32 bytes) stops the service at startup.
	internalToken, internalProblem := sharedconfig.LoadInternalToken(loader, appEnv)
	if internalProblem != "" && appEnv != "development" && loader.String("INTERNAL_TOKEN", "") != "" {
		return Config{}, errors.New(internalProblem)
	}

	frontend := strings.TrimRight(loader.String("FRONTEND_BASE_URL", "http://localhost:8080"), "/")
	if !validHTTPOrigin(frontend) {
		return Config{}, errors.New("FRONTEND_BASE_URL must be an absolute http or https URL")
	}
	identityURL := strings.TrimRight(loader.String("IDENTITY_BASE_URL", "http://identity:8081"), "/")
	if !validHTTPOrigin(identityURL) {
		return Config{}, errors.New("IDENTITY_BASE_URL must be an HTTP(S) service origin")
	}
	pricingURL := strings.TrimRight(loader.String("PRICING_BASE_URL", "http://pricing:8083"), "/")
	if !validHTTPOrigin(pricingURL) {
		return Config{}, errors.New("PRICING_BASE_URL must be an HTTP(S) service origin")
	}
	identityTimeout, err := loader.Duration("IDENTITY_TIMEOUT", 3*time.Second)
	if err != nil {
		return Config{}, err
	}
	if identityTimeout < time.Millisecond {
		return Config{}, errors.New("IDENTITY_TIMEOUT must be at least 1ms")
	}
	pricingTimeout, err := loader.Duration("PRICING_TIMEOUT", 3*time.Second)
	if err != nil {
		return Config{}, err
	}
	if pricingTimeout < time.Millisecond {
		return Config{}, errors.New("PRICING_TIMEOUT must be at least 1ms")
	}

	parcelURL := strings.TrimRight(loader.String("PARCEL_BASE_URL", "http://parcel:8082"), "/")
	if !validHTTPOrigin(parcelURL) {
		return Config{}, errors.New("PARCEL_BASE_URL must be an HTTP(S) service origin")
	}
	parcelTimeout, err := loader.Duration("PARCEL_TIMEOUT", 3*time.Second)
	if err != nil {
		return Config{}, err
	}
	if parcelTimeout < time.Millisecond {
		return Config{}, errors.New("PARCEL_TIMEOUT must be at least 1ms")
	}

	rabbitMQURL := strings.TrimSpace(loader.String("RABBITMQ_URL", ""))
	if rabbitMQURL != "" {
		if err := sharedconfig.ValidateAMQPURL(rabbitMQURL); err != nil {
			return Config{}, fmt.Errorf("RABBITMQ_URL: %w", err)
		}
	}
	exchange := strings.TrimSpace(loader.String("RABBITMQ_EXCHANGE", "swiftdrop.events"))
	if exchange == "" {
		return Config{}, errors.New("RABBITMQ_EXCHANGE must not be empty")
	}
	syncQueue := strings.TrimSpace(loader.String("MERCHANT_SYNC_QUEUE", "merchant.sync.queue"))
	if syncQueue == "" {
		return Config{}, errors.New("MERCHANT_SYNC_QUEUE must not be empty")
	}
	syncRoutingKey := strings.TrimSpace(loader.String("MERCHANT_SYNC_ROUTING_KEY", "parcel.status_changed.v1"))
	if syncRoutingKey == "" {
		return Config{}, errors.New("MERCHANT_SYNC_ROUTING_KEY must not be empty")
	}
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
	if handlerTimeout <= 0 {
		return Config{}, errors.New("RABBITMQ_HANDLER_TIMEOUT must be greater than zero")
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
		Port:             port,
		AppEnv:           appEnv,
		LogLevel:         logLevel,
		DatabaseURL:      loader.String("DATABASE_URL", ""),
		AccessSecret:     accessSecret,
		CookieName:       loader.String("COOKIE_NAME", "sd_access"),
		WebhookSecretKey: webhookSecretKey,

		InternalToken:        internalToken,
		InternalTokenProblem: internalProblem,
		FrontendBaseURL:      frontend,
		SupportEmail:         loader.String("SUPPORT_EMAIL", ""),
		IdentityBaseURL:      identityURL,
		IdentityTimeout:      identityTimeout,
		PricingBaseURL:       pricingURL,
		PricingTimeout:       pricingTimeout,
		ParcelBaseURL:        parcelURL,
		ParcelTimeout:        parcelTimeout,

		RabbitMQURL:            rabbitMQURL,
		RabbitMQExchange:       exchange,
		MerchantSyncQueue:      syncQueue,
		MerchantSyncRoutingKey: syncRoutingKey,
		RabbitMQPrefetch:       prefetch,
		RabbitMQHandlerTimeout: handlerTimeout,
		RabbitMQMaxRetries:     maxRetries,
		RabbitMQRetryDelay:     retryDelay,
		RabbitMQReconnectMin:   reconnectMin,
		RabbitMQReconnectMax:   reconnectMax,
	}, nil
}

func validHTTPOrigin(value string) bool {
	parsed, err := url.Parse(value)
	return err == nil && parsed.Host != "" && (parsed.Scheme == "http" || parsed.Scheme == "https")
}

func (c Config) Addr() string {
	return ":" + strconv.Itoa(c.Port)
}
