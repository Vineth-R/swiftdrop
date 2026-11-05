package config

import (
	"fmt"
	"strconv"

	sharedconfig "github.com/Vineth-R/swiftdrop/packages/go-common/config"
)

// MinInternalTokenLength is the development minimum. Validation itself lives
// in the shared config package so identity and pricing apply identical rules.
const MinInternalTokenLength = sharedconfig.MinInternalTokenLength

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
	appEnv, err := sharedconfig.LoadAppEnv(loader)
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

	internalToken, problem := sharedconfig.LoadInternalToken(loader, appEnv)
	if problem != "" && appEnv != "development" {
		return Config{}, fmt.Errorf("load internal token: %s", problem)
	}

	accessSecret, err := sharedconfig.LoadAccessSecret(loader, appEnv, false)
	if err != nil {
		return Config{}, err
	}

	return Config{
		Port:           port,
		AppEnv:         appEnv,
		LogLevel:       logLevel,
		DatabaseURL:    loader.String("DATABASE_URL", ""),
		InternalToken:  internalToken,
		AccessSecret:   accessSecret,
		CookieName:     loader.String("COOKIE_NAME", "sd_access"),
		TrustedProxies: proxies,
	}, nil
}

func (c Config) Addr() string {
	return ":" + strconv.Itoa(c.Port)
}
