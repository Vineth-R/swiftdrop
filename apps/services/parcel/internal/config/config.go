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
	Port                 int
	AppEnv               string
	LogLevel             string
	DatabaseURL          string
	TrustedProxies       []string
	AccessSecret         string
	CookieName           string
	InternalToken        string
	InternalTokenProblem string
	PricingBaseURL       string
	PricingTimeout       time.Duration
	IdentityBaseURL      string
	IdentityTimeout      time.Duration
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
	internalToken, problem := sharedconfig.LoadInternalToken(loader, appEnv)
	if problem != "" && appEnv != "development" && loader.String("INTERNAL_TOKEN", "") != "" {
		return Config{}, errors.New(problem)
	}
	pricingURL := strings.TrimRight(loader.String("PRICING_BASE_URL", "http://pricing:8083"), "/")
	u, err := url.Parse(pricingURL)
	if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return Config{}, errors.New("PRICING_BASE_URL must be an HTTP(S) service origin")
	}
	pricingTimeout, err := loader.Duration("PRICING_TIMEOUT", 3*time.Second)
	if err != nil {
		return Config{}, err
	}
	if pricingTimeout < time.Millisecond {
		return Config{}, errors.New("PRICING_TIMEOUT must be at least 1ms")
	}
	identityURL := strings.TrimRight(loader.String("IDENTITY_BASE_URL", "http://identity:8081"), "/")
	u, err = url.Parse(identityURL)
	if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return Config{}, errors.New("IDENTITY_BASE_URL must be an HTTP(S) service origin")
	}
	identityTimeout, err := loader.Duration("IDENTITY_TIMEOUT", 3*time.Second)
	if err != nil {
		return Config{}, err
	}
	if identityTimeout < time.Millisecond {
		return Config{}, errors.New("IDENTITY_TIMEOUT must be at least 1ms")
	}

	return Config{
		Port:                 port,
		AppEnv:               appEnv,
		LogLevel:             logLevel,
		DatabaseURL:          loader.String("DATABASE_URL", ""),
		TrustedProxies:       proxies,
		AccessSecret:         accessSecret,
		CookieName:           loader.String("COOKIE_NAME", "sd_access"),
		InternalToken:        internalToken,
		InternalTokenProblem: problem,
		PricingBaseURL:       pricingURL,
		PricingTimeout:       pricingTimeout,
		IdentityBaseURL:      identityURL,
		IdentityTimeout:      identityTimeout,
	}, nil
}

func (c Config) Addr() string {
	return ":" + strconv.Itoa(c.Port)
}
