package config

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	sharedconfig "github.com/Vineth-R/swiftdrop/packages/go-common/config"
)

// developmentOrigins are the browser origins allowed to open a WebSocket in
// development: the Next.js dev server, which is served on a different port
// than the gateway.
const developmentOrigins = "localhost:3000,127.0.0.1:3000"

type Config struct {
	Port        int
	AppEnv      string
	LogLevel    string
	DatabaseURL string

	// AccessSecret verifies access tokens. When it is empty every connection is
	// treated as anonymous: following one parcel by tracking number still works,
	// but following everything (scope all) is refused (fail closed).
	AccessSecret string
	CookieName   string

	// AllowedOrigins are extra browser origins (host or host:port patterns)
	// allowed to open a WebSocket. The page's own host is always allowed.
	AllowedOrigins []string

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

	port, err := loader.Port("PORT", 8085)
	if err != nil {
		return Config{}, err
	}

	accessSecret, err := sharedconfig.LoadAccessSecret(loader, appEnv, false)
	if err != nil {
		return Config{}, err
	}

	originsDefault := ""
	if appEnv == "development" {
		originsDefault = developmentOrigins
	}
	allowedOrigins, err := parseOrigins(loader.String("REALTIME_ALLOWED_ORIGINS", originsDefault))
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

	// The loader trims values, and a blank value falls back to the default.
	queue := loader.String("REALTIME_QUEUE", "realtime.parcel.status_changed.v1")
	routingKey := loader.String("REALTIME_ROUTING_KEY", "parcel.status_changed.v1")

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
		Port:           port,
		AppEnv:         appEnv,
		LogLevel:       logLevel,
		DatabaseURL:    loader.String("DATABASE_URL", ""),
		AccessSecret:   accessSecret,
		CookieName:     loader.String("COOKIE_NAME", "sd_access"),
		AllowedOrigins: allowedOrigins,
		RabbitMQURL:    rabbitMQURL,
		Exchange:       loader.String("RABBITMQ_EXCHANGE", "swiftdrop.events"),
		Queue:          queue,
		RoutingKey:     routingKey,
		Prefetch:       prefetch,
		HandlerTimeout: handlerTimeout,
		MaxRetries:     maxRetries,
		RetryDelay:     retryDelay,
		ReconnectMin:   reconnectMin,
		ReconnectMax:   reconnectMax,
	}, nil
}

// parseOrigins splits a comma-separated list of host patterns. A pattern is a
// host or host:port, never a URL, and never a bare wildcard that would allow
// every site.
func parseOrigins(value string) ([]string, error) {
	var origins []string
	for _, part := range strings.Split(value, ",") {
		origin := strings.TrimSpace(part)
		if origin == "" {
			continue
		}
		if origin == "*" || strings.Contains(origin, "://") || strings.ContainsAny(origin, "/ ") {
			return nil, fmt.Errorf("REALTIME_ALLOWED_ORIGINS entry %q must be a host or host:port, not a URL or wildcard", origin)
		}
		origins = append(origins, origin)
	}
	return origins, nil
}

func (c Config) Addr() string {
	return ":" + strconv.Itoa(c.Port)
}
