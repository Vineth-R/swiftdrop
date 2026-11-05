package config

import (
	"errors"
	"fmt"
	"net/mail"
	"net/url"
	"strconv"
	"strings"
	"time"

	sharedconfig "github.com/Vineth-R/swiftdrop/packages/go-common/config"
	"github.com/Vineth-R/swiftdrop/packages/go-common/mailer"
)

const (
	defaultProcessedEventsRetention       = 30 * 24 * time.Hour
	defaultProcessedEventsCleanupInterval = 24 * time.Hour
)

type Config struct {
	Port                           int
	AppEnv                         string
	LogLevel                       string
	DatabaseURL                    string
	SMTP                           mailer.Config
	RabbitMQURL                    string
	Exchange                       string
	Queue                          string
	RoutingKey                     string
	Prefetch                       int
	HandlerTimeout                 time.Duration
	MaxRetries                     int
	RetryDelay                     time.Duration
	ReconnectMin                   time.Duration
	ReconnectMax                   time.Duration
	AccessSecret                   string
	CookieName                     string
	ProcessedEventsRetention       time.Duration
	ProcessedEventsCleanupInterval time.Duration
	IdentityBaseURL                string
	IdentityTimeout                time.Duration
	InternalToken                  string
	InternalTokenProblem           string
	ParcelCreatedQueue             string
	ParcelCreatedRoutingKey        string
	ParcelStatusQueue              string
	ParcelStatusRoutingKey         string
	FrontendBaseURL                string
	SupportEmail                   string
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

	port, err := loader.Port("PORT", 8086)
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

	databaseURL := loader.String("DATABASE_URL", "")
	if databaseURL == "" {
		return Config{}, errors.New("DATABASE_URL is required")
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

	retention, err := loader.Duration("NOTIFICATION_PROCESSED_EVENTS_RETENTION", defaultProcessedEventsRetention)
	if err != nil {
		return Config{}, err
	}

	cleanupInterval, err := loader.Duration("NOTIFICATION_PROCESSED_EVENTS_CLEANUP_INTERVAL", defaultProcessedEventsCleanupInterval)
	if err != nil {
		return Config{}, err
	}

	accessSecret, err := sharedconfig.LoadAccessSecret(loader, appEnv, false)
	if err != nil {
		return Config{}, err
	}

	identityBaseURL := strings.TrimRight(strings.TrimSpace(loader.String("IDENTITY_BASE_URL", "http://identity:8081")), "/")
	if err := validateBaseURL(identityBaseURL); err != nil {
		return Config{}, fmt.Errorf("IDENTITY_BASE_URL: %w", err)
	}

	identityTimeout, err := loader.Duration("IDENTITY_TIMEOUT", 3*time.Second)
	if err != nil {
		return Config{}, err
	}
	if identityTimeout < time.Millisecond {
		return Config{}, errors.New("IDENTITY_TIMEOUT must be at least 1ms")
	}

	internalToken, internalTokenProblem := sharedconfig.LoadInternalToken(loader, appEnv)

	routingKey := loader.String("NOTIFICATION_ROUTING_KEY", queue)
	parcelCreatedQueue := strings.TrimSpace(loader.String("NOTIFICATION_PARCEL_CREATED_QUEUE", "notification.parcel.created.v1"))
	parcelCreatedRoutingKey := strings.TrimSpace(loader.String("NOTIFICATION_PARCEL_CREATED_ROUTING_KEY", "parcel.created.v1"))
	parcelStatusQueue := strings.TrimSpace(loader.String("NOTIFICATION_PARCEL_STATUS_QUEUE", "notification.parcel.status_changed.v1"))
	parcelStatusRoutingKey := strings.TrimSpace(loader.String("NOTIFICATION_PARCEL_STATUS_ROUTING_KEY", "parcel.status_changed.v1"))
	for name, value := range map[string]string{
		"NOTIFICATION_PARCEL_CREATED_QUEUE":       parcelCreatedQueue,
		"NOTIFICATION_PARCEL_CREATED_ROUTING_KEY": parcelCreatedRoutingKey,
		"NOTIFICATION_PARCEL_STATUS_QUEUE":        parcelStatusQueue,
		"NOTIFICATION_PARCEL_STATUS_ROUTING_KEY":  parcelStatusRoutingKey,
	} {
		if value == "" {
			return Config{}, fmt.Errorf("%s must not be empty", name)
		}
	}
	if parcelCreatedQueue == queue || parcelStatusQueue == queue || parcelCreatedQueue == parcelStatusQueue {
		return Config{}, errors.New("notification queues must be distinct")
	}
	routingKeys := []struct {
		name  string
		value string
	}{
		{"NOTIFICATION_ROUTING_KEY", routingKey},
		{"NOTIFICATION_PARCEL_CREATED_ROUTING_KEY", parcelCreatedRoutingKey},
		{"NOTIFICATION_PARCEL_STATUS_ROUTING_KEY", parcelStatusRoutingKey},
	}
	for i := range routingKeys {
		for j := i + 1; j < len(routingKeys); j++ {
			if routingKeys[i].value == routingKeys[j].value {
				return Config{}, fmt.Errorf("notification routing keys must be distinct: %s conflicts with %s", routingKeys[i].name, routingKeys[j].name)
			}
		}
	}

	frontendBaseURL := strings.TrimRight(strings.TrimSpace(loader.String("FRONTEND_BASE_URL", "http://localhost:8080")), "/")
	if err := validateBaseURL(frontendBaseURL); err != nil {
		return Config{}, fmt.Errorf("FRONTEND_BASE_URL: %w", err)
	}

	supportEmail := strings.TrimSpace(loader.String("SUPPORT_EMAIL", ""))
	if supportEmail != "" {
		address, err := mail.ParseAddress(supportEmail)
		if err != nil || address.Address != supportEmail {
			return Config{}, errors.New("SUPPORT_EMAIL must be a plain email address")
		}
	}

	return Config{
		Port:                           port,
		AppEnv:                         appEnv,
		LogLevel:                       logLevel,
		DatabaseURL:                    databaseURL,
		SMTP:                           smtp,
		RabbitMQURL:                    rabbitMQURL,
		Exchange:                       loader.String("RABBITMQ_EXCHANGE", "swiftdrop.events"),
		Queue:                          queue,
		RoutingKey:                     routingKey,
		Prefetch:                       prefetch,
		HandlerTimeout:                 handlerTimeout,
		MaxRetries:                     maxRetries,
		RetryDelay:                     retryDelay,
		ReconnectMin:                   reconnectMin,
		ReconnectMax:                   reconnectMax,
		AccessSecret:                   accessSecret,
		CookieName:                     loader.String("COOKIE_NAME", "sd_access"),
		ProcessedEventsRetention:       retention,
		ProcessedEventsCleanupInterval: cleanupInterval,
		IdentityBaseURL:                identityBaseURL,
		IdentityTimeout:                identityTimeout,
		InternalToken:                  internalToken,
		InternalTokenProblem:           internalTokenProblem,
		ParcelCreatedQueue:             parcelCreatedQueue,
		ParcelCreatedRoutingKey:        parcelCreatedRoutingKey,
		ParcelStatusQueue:              parcelStatusQueue,
		ParcelStatusRoutingKey:         parcelStatusRoutingKey,
		FrontendBaseURL:                frontendBaseURL,
		SupportEmail:                   supportEmail,
	}, nil
}

func validateBaseURL(value string) error {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return errors.New("must be an absolute http or https URL")
	}
	return nil
}

// Addr returns the HTTP listen address.
func (c Config) Addr() string {
	return ":" + strconv.Itoa(c.Port)
}
