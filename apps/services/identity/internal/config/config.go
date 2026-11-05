package config

import (
	"strconv"

	sharedconfig "github.com/Vineth-R/swiftdrop/packages/go-common/config"
)

type Config struct {
	Port               int
	AppEnv             string
	LogLevel           string
	DatabaseURL        string
	GoogleClientID     string
	GoogleClientSecret string
	GoogleRedirectURI  string
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

	port, err := loader.Port("PORT", 8081)
	if err != nil {
		return Config{}, err
	}

	return Config{
		Port:               port,
		AppEnv:             appEnv,
		LogLevel:           logLevel,
		DatabaseURL:        loader.String("DATABASE_URL", ""),
		GoogleClientID:     loader.String("GOOGLE_CLIENT_ID", ""),
		GoogleClientSecret: loader.String("GOOGLE_CLIENT_SECRET", ""),
		GoogleRedirectURI:  loader.String("GOOGLE_REDIRECT_URI", ""),
	}, nil
}

func (c Config) Addr() string {
	return ":" + strconv.Itoa(c.Port)
}
