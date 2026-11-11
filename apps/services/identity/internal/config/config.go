package config

import (
	"errors"
	"net/url"
	"strconv"
	"strings"
	"time"

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
	MerchantBaseURL    string
	MerchantTimeout    time.Duration
	InternalToken      string
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

	merchantURL := strings.TrimRight(loader.String("MERCHANT_BASE_URL", "http://merchant:8084"), "/")
	parsed, err := url.Parse(merchantURL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return Config{}, errors.New("MERCHANT_BASE_URL must be an HTTP(S) service origin")
	}
	internalToken, _ := sharedconfig.LoadInternalToken(loader, appEnv)
	merchantTimeout, err := loader.Duration("MERCHANT_TIMEOUT", 3*time.Second)
	if err != nil {
		return Config{}, err
	}
	if merchantTimeout < time.Millisecond {
		return Config{}, errors.New("MERCHANT_TIMEOUT must be at least 1ms")
	}

	return Config{
		Port:               port,
		AppEnv:             appEnv,
		LogLevel:           logLevel,
		DatabaseURL:        loader.String("DATABASE_URL", ""),
		GoogleClientID:     loader.String("GOOGLE_CLIENT_ID", ""),
		GoogleClientSecret: loader.String("GOOGLE_CLIENT_SECRET", ""),
		GoogleRedirectURI:  loader.String("GOOGLE_REDIRECT_URI", ""),
		MerchantBaseURL:    merchantURL,
		MerchantTimeout:    merchantTimeout,
		InternalToken:      internalToken,
	}, nil
}

func (c Config) Addr() string {
	return ":" + strconv.Itoa(c.Port)
}
