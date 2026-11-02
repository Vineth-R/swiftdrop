package config

import (
	"strconv"

	sharedconfig "github.com/Vineth-R/swiftdrop/packages/go-common/config"
)

// MinInternalTokenLength matches the identity service. An unset or shorter
// token is treated as not configured so internal endpoints stay fail-closed.
const MinInternalTokenLength = 16

type Config struct {
	Port           int
	AppEnv         string
	LogLevel       string
	DatabaseURL    string
	InternalToken  string
	AccessSecret   string
	CookieName     string
	TrustedProxies []string
}

func Load() (Config, error) {
	return LoadFrom(sharedconfig.Load())
}

func LoadFrom(loader sharedconfig.Loader) (Config, error) {
	appEnv, err := loader.EnumFold("APP_ENV", "development", "development", "staging", "production")
	if err != nil {
		return Config{}, err
	}

	logLevel, err := loader.EnumFold("LOG_LEVEL", "info", "debug", "info", "warn", "warning", "error")
	if err != nil {
		return Config{}, err
	}

	port, err := loader.Port("PORT", 8083)
	if err != nil {
		return Config{}, err
	}
	proxies, err := loader.CIDRs("HTTP_TRUSTED_PROXIES")
	if err != nil {
		return Config{}, err
	}

	internalToken := loader.String("INTERNAL_TOKEN", "")
	if len(internalToken) < MinInternalTokenLength {
		internalToken = ""
	}

	return Config{
		Port:           port,
		AppEnv:         appEnv,
		LogLevel:       logLevel,
		DatabaseURL:    loader.String("DATABASE_URL", ""),
		InternalToken:  internalToken,
		AccessSecret:   loader.String("JWT_ACCESS_SECRET", ""),
		CookieName:     loader.String("COOKIE_NAME", "sd_access"),
		TrustedProxies: proxies,
	}, nil
}

func (c Config) Addr() string {
	return ":" + strconv.Itoa(c.Port)
}
