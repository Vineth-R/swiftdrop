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

	// Pool settings accept the DB_* names first and the legacy POSTGRES_*
	// names as aliases so existing local and Compose configuration keeps working.
	envMaxOpenConns     = "DB_MAX_OPEN_CONNS"
	envLegacyMaxConns   = "POSTGRES_MAX_CONNS"
	envMaxIdleConns     = "DB_MAX_IDLE_CONNS"
	envLegacyMinConns   = "POSTGRES_MIN_CONNS"
	envConnMaxLifetime  = "DB_CONN_MAX_LIFETIME"
	envConnMaxIdleTime  = "DB_CONN_MAX_IDLE_TIME"
	envConnectTimeout   = "DB_CONNECT_TIMEOUT"
	envHealthCheck      = "DB_HEALTH_CHECK_PERIOD"
	neonHostnameSuffix  = ".neon.tech"
	strongSSLMode       = "require"
	strongSSLModeVerify = "verify-ca"
	strongSSLModeFull   = "verify-full"
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

// LoadConfigFromURL builds a pool configuration for a URL already read by a
// service. Pool limits still come from the environment, so every service uses
// the same DB_* (and legacy POSTGRES_*) settings. An empty URL falls back to
// the regular DATABASE_URL/POSTGRES_* resolution.
func LoadConfigFromURL(rawURL string) (Config, error) {
	return loadConfig(sharedconfig.Load(), rawURL)
}

func LoadConfig(loader sharedconfig.Loader) (Config, error) {
	return loadConfig(loader, "")
}

func loadConfig(loader sharedconfig.Loader, explicitURL string) (Config, error) {
	config := DefaultConfig()

	config.URL = strings.TrimSpace(explicitURL)
	if config.URL == "" {
		config.URL = loader.String("DATABASE_URL", "")
	}
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

	pool := poolSettings(loader)

	maxConns, err := loader.Int(pool.maxConnsKey, DefaultMaxConns)
	if err != nil {
		return Config{}, err
	}
	if maxConns < 1 {
		return Config{}, fmt.Errorf("%w: %s must be at least 1", ErrInvalidConfig, envMaxOpenConns)
	}
	config.MaxConns = int32(maxConns)

	minConns, err := loader.Int(pool.minConnsKey, DefaultMinConns)
	if err != nil {
		return Config{}, err
	}
	if minConns < 0 || minConns > maxConns {
		return Config{}, fmt.Errorf("%w: %s must be between 0 and %s", ErrInvalidConfig, envMaxIdleConns, envMaxOpenConns)
	}
	config.MinConns = int32(minConns)

	maxLifetime, err := loader.Duration(pool.maxLifetimeKey, DefaultMaxConnLifetime)
	if err != nil {
		return Config{}, err
	}
	config.MaxConnLifetime = maxLifetime

	maxIdle, err := loader.Duration(pool.maxIdleTimeKey, DefaultMaxConnIdleTime)
	if err != nil {
		return Config{}, err
	}
	config.MaxConnIdleTime = maxIdle

	healthCheck, err := loader.Duration(pool.healthCheckKey, DefaultHealthCheck)
	if err != nil {
		return Config{}, err
	}
	config.HealthCheckPeriod = healthCheck

	connectTimeout, err := loader.Duration(pool.connectTimeoutKey, DefaultConnectTimeout)
	if err != nil {
		return Config{}, err
	}
	config.ConnectTimeout = connectTimeout

	if err := validateURL(config.URL); err != nil {
		return Config{}, err
	}

	return config, nil
}

type poolKeys struct {
	maxConnsKey       string
	minConnsKey       string
	maxLifetimeKey    string
	maxIdleTimeKey    string
	healthCheckKey    string
	connectTimeoutKey string
}

func poolSettings(loader sharedconfig.Loader) poolKeys {
	return poolKeys{
		maxConnsKey:       firstSet(loader, envMaxOpenConns, envLegacyMaxConns),
		minConnsKey:       firstSet(loader, envMaxIdleConns, envLegacyMinConns),
		maxLifetimeKey:    envConnMaxLifetime,
		maxIdleTimeKey:    envConnMaxIdleTime,
		healthCheckKey:    envHealthCheck,
		connectTimeoutKey: envConnectTimeout,
	}
}

// firstSet returns the first key with a non-empty value, or fallback when none
// is set. This keeps DB_* canonical while accepting the legacy names.
func firstSet(loader sharedconfig.Loader, keys ...string) string {
	for _, key := range keys {
		if loader.String(key, "") != "" {
			return key
		}
	}
	return keys[0]
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

	if IsNeonHost(host) {
		sslmode := strings.ToLower(strings.TrimSpace(parsed.Query().Get("sslmode")))
		switch sslmode {
		case strongSSLMode, strongSSLModeVerify, strongSSLModeFull:
		default:
			return fmt.Errorf("%w: DATABASE_URL must require SSL for Neon hosts", ErrInvalidConfig)
		}
	}

	return nil
}

// IsNeonHost reports whether a database host belongs to Neon. Neon rejects
// unencrypted and opportunistic TLS modes, so configuration fails closed.
func IsNeonHost(host string) bool {
	return strings.HasSuffix(strings.ToLower(strings.TrimSpace(host)), neonHostnameSuffix)
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
