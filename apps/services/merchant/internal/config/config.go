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
	Port             int
	AppEnv           string
	LogLevel         string
	DatabaseURL      string
	AccessSecret     string
	CookieName       string
	WebhookSecretKey string

	// InternalToken guards the /internal endpoints. Empty keeps them closed:
	// every call is refused. InternalTokenProblem says why it is empty.
	InternalToken        string
	InternalTokenProblem string

	FrontendBaseURL string
	SupportEmail    string
	IdentityBaseURL string
	IdentityTimeout time.Duration
	PricingBaseURL  string
	PricingTimeout  time.Duration
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

	frontend := strings.TrimRight(loader.String("FRONTEND_BASE_URL", "http://localhost:8080"), "/")
	if !validHTTPOrigin(frontend) {
		return Config{}, errors.New("FRONTEND_BASE_URL must be an absolute http or https URL")
	}
	identityURL := strings.TrimRight(loader.String("IDENTITY_BASE_URL", "http://identity:8081"), "/")
	if !validHTTPOrigin(identityURL) {
		return Config{}, errors.New("IDENTITY_BASE_URL must be an HTTP(S) service origin")
	}
	pricingURL := strings.TrimRight(loader.String("PRICING_BASE_URL", "http://pricing:8083"), "/")
	if !validHTTPOrigin(pricingURL) {
		return Config{}, errors.New("PRICING_BASE_URL must be an HTTP(S) service origin")
	}
	identityTimeout, err := loader.Duration("IDENTITY_TIMEOUT", 3*time.Second)
	if err != nil {
		return Config{}, err
	}
	if identityTimeout < time.Millisecond {
		return Config{}, errors.New("IDENTITY_TIMEOUT must be at least 1ms")
	}
	pricingTimeout, err := loader.Duration("PRICING_TIMEOUT", 3*time.Second)
	if err != nil {
		return Config{}, err
	}
	if pricingTimeout < time.Millisecond {
		return Config{}, errors.New("PRICING_TIMEOUT must be at least 1ms")
	}

	return Config{
		Port:             port,
		AppEnv:           appEnv,
		LogLevel:         logLevel,
		DatabaseURL:      loader.String("DATABASE_URL", ""),
		AccessSecret:     accessSecret,
		CookieName:       loader.String("COOKIE_NAME", "sd_access"),
		WebhookSecretKey: webhookSecretKey,

		InternalToken:        internalToken,
		InternalTokenProblem: internalProblem,
		FrontendBaseURL:      frontend,
		SupportEmail:         loader.String("SUPPORT_EMAIL", ""),
		IdentityBaseURL:      identityURL,
		IdentityTimeout:      identityTimeout,
		PricingBaseURL:       pricingURL,
		PricingTimeout:       pricingTimeout,
	}, nil
}

func validHTTPOrigin(value string) bool {
	parsed, err := url.Parse(value)
	return err == nil && parsed.Host != "" && (parsed.Scheme == "http" || parsed.Scheme == "https")
}

func (c Config) Addr() string {
	return ":" + strconv.Itoa(c.Port)
}
