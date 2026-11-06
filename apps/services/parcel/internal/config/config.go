package config

import (
	"strconv"
	"time"

	sharedconfig "github.com/Vineth-R/swiftdrop/packages/go-common/config"
)

type Config struct {
	Port             int
	AppEnv           string
	LogLevel         string
	DatabaseURL      string
	TrustedProxies   []string
	AccessSecret     string
	CookieName       string
	IdentityBaseURL  string
	IdentityTimeout  time.Duration
	InternalToken    string
	RabbitMQURL      string
	RabbitMQExchange string
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

	port, err := loader.Port("PORT", 8082)
	if err != nil {
		return Config{}, err
	}
	proxies, err := loader.CIDRs("HTTP_TRUSTED_PROXIES")
	if err != nil {
		return Config{}, err
	}

	accessSecret, err := sharedconfig.LoadAccessSecret(loader, appEnv, false)
	if err != nil {
		return Config{}, err
	}

	identityTimeout, _ := loader.Duration("IDENTITY_TIMEOUT", 3*time.Second)
	internalToken, _ := sharedconfig.LoadInternalToken(loader, appEnv)

	return Config{
		Port:             port,
		AppEnv:           appEnv,
		LogLevel:         logLevel,
		DatabaseURL:      loader.String("DATABASE_URL", ""),
		TrustedProxies:   proxies,
		AccessSecret:     accessSecret,
		CookieName:       loader.String("COOKIE_NAME", "sd_access"),
		IdentityBaseURL:  loader.String("IDENTITY_BASE_URL", "http://identity:8081"),
		IdentityTimeout:  identityTimeout,
		InternalToken:    internalToken,
		RabbitMQURL:      loader.String("RABBITMQ_URL", ""),
		RabbitMQExchange: loader.String("RABBITMQ_EXCHANGE", "swiftdrop.events"),
	}, nil
}

func (c Config) Addr() string {
	return ":" + strconv.Itoa(c.Port)
}
