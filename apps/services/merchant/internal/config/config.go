package config

import (
	"errors"
	"strconv"

	sharedconfig "github.com/Vineth-R/swiftdrop/packages/go-common/config"
)

type Config struct {
	Port        int
	AppEnv      string
	LogLevel    string
	DatabaseURL string
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

	return Config{
		Port:        port,
		AppEnv:      appEnv,
		LogLevel:    logLevel,
		DatabaseURL: loader.String("DATABASE_URL", ""),
	}, nil
}

func (c Config) Addr() string {
	return ":" + strconv.Itoa(c.Port)
}
