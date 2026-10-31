package postgres

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	sharedconfig "github.com/Vineth-R/swiftdrop/packages/go-common/config"
)

const (
	DefaultMaxConns         = 10
	DefaultMinConns         = 2
	DefaultMaxConnLifetime  = 30 * time.Minute
	DefaultMaxConnIdleTime  = 5 * time.Minute
	DefaultHealthCheck      = time.Minute
	DefaultConnectTimeout   = 5 * time.Second
	DefaultReadyTimeout     = 2 * time.Second
	DefaultRetryAttempts    = 5
	DefaultRetryMin         = 250 * time.Millisecond
	DefaultRetryMax         = 5 * time.Second
	DefaultPostgresPort     = 5432
	DefaultPostgresSSLMode  = "disable"
	DefaultPostgresHostname = "localhost"
)

var (
	ErrNotConfigured = errors.New("database is not configured")
	ErrInvalidConfig = errors.New("invalid database configuration")
)

type Config struct {
	URL               string
	MaxConns          int32
	MinConns          int32
	MaxConnLifetime   time.Duration
	MaxConnIdleTime   time.Duration
	HealthCheckPeriod time.Duration
	ConnectTimeout    time.Duration
	ReadyTimeout      time.Duration
	RetryAttempts     int
	RetryMin          time.Duration
	RetryMax          time.Duration
}

func (c Config) Configured() bool {
	return strings.TrimSpace(c.URL) != ""
}

func DefaultConfig() Config {
	return Config{
		MaxConns:          DefaultMaxConns,
		MinConns:          DefaultMinConns,
		MaxConnLifetime:   DefaultMaxConnLifetime,
		MaxConnIdleTime:   DefaultMaxConnIdleTime,
		HealthCheckPeriod: DefaultHealthCheck,
		ConnectTimeout:    DefaultConnectTimeout,
		ReadyTimeout:      DefaultReadyTimeout,
		RetryAttempts:     DefaultRetryAttempts,
		RetryMin:          DefaultRetryMin,
		RetryMax:          DefaultRetryMax,
	}
}

func LoadConfigFromEnv() (Config, error) {
	return LoadConfig(sharedconfig.Load())
}

func LoadConfig(loader sharedconfig.Loader) (Config, error) {
	config := DefaultConfig()

	config.URL = loader.String("DATABASE_URL", "")
	if !config.Configured() {
		host := loader.String("POSTGRES_HOST", "")
		if host == "" {
			return config, nil
		}

		port, err := loader.Port("POSTGRES_PORT", DefaultPostgresPort)
		if err != nil {
			return Config{}, err
		}

		user := loader.String("POSTGRES_USER", "")
		database := loader.String("POSTGRES_DB", "")
		if user == "" || database == "" {
			return Config{}, fmt.Errorf("%w: POSTGRES_USER and POSTGRES_DB are required when DATABASE_URL is not set", ErrInvalidConfig)
		}

		password := loader.String("POSTGRES_PASSWORD", "")
		sslmode := loader.String("POSTGRES_SSLMODE", DefaultPostgresSSLMode)

		config.URL = buildURL(host, port, user, password, database, sslmode)
	}

	maxConns, err := loader.Int("POSTGRES_MAX_CONNS", DefaultMaxConns)
	if err != nil {
		return Config{}, err
	}
	if maxConns < 1 {
		return Config{}, fmt.Errorf("%w: POSTGRES_MAX_CONNS must be at least 1", ErrInvalidConfig)
	}
	config.MaxConns = int32(maxConns)

	minConns, err := loader.Int("POSTGRES_MIN_CONNS", DefaultMinConns)
	if err != nil {
		return Config{}, err
	}
	if minConns < 0 || minConns > maxConns {
		return Config{}, fmt.Errorf("%w: POSTGRES_MIN_CONNS must be between 0 and POSTGRES_MAX_CONNS", ErrInvalidConfig)
	}
	config.MinConns = int32(minConns)

	if err := validateURL(config.URL); err != nil {
		return Config{}, err
	}

	return config, nil
}

func validateURL(raw string) error {
	if strings.TrimSpace(raw) == "" {
		return nil
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("%w: DATABASE_URL is not a valid URL", ErrInvalidConfig)
	}

	switch parsed.Scheme {
	case "postgres", "postgresql":
	default:
		return fmt.Errorf("%w: DATABASE_URL must use the postgres or postgresql scheme", ErrInvalidConfig)
	}

	host := parsed.Hostname()
	if host == "" {
		return fmt.Errorf("%w: DATABASE_URL must include a host", ErrInvalidConfig)
	}
	if parsed.Port() != "" {
		if _, err := net.LookupPort("tcp", parsed.Port()); err != nil {
			return fmt.Errorf("%w: DATABASE_URL must include a valid port", ErrInvalidConfig)
		}
	}

	return nil
}

func buildURL(host string, port int, user, password, database, sslmode string) string {
	escaped := url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(user, password),
		Host:     net.JoinHostPort(host, strconv.Itoa(port)),
		Path:     "/" + database,
		RawQuery: "sslmode=" + url.QueryEscape(sslmode),
	}
	return escaped.String()
}
