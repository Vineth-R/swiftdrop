package config

import (
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
	appEnv, err := loader.Enum("APP_ENV", "development", "development", "staging", "production")
	if err != nil {
		return Config{}, err
	}

	logLevel, err := loader.Enum("LOG_LEVEL", "info", "debug", "info", "warn", "error")
	if err != nil {
		return Config{}, err
	}

	port, err := loader.Port("PORT", 8084)
	if err != nil {
		return Config{}, err
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
