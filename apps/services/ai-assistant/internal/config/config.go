package config

import (
	"errors"
	"fmt"
	"math"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	sharedconfig "github.com/Vineth-R/swiftdrop/packages/go-common/config"
)

const defaultModel = "gemini-3.5-flash"

var cookieNamePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// Config is the complete ai-assistant runtime configuration.
type Config struct {
	Port           int
	AppEnv         string
	LogLevel       string
	TrustedProxies []string

	AccessSecret string
	CookieName   string

	GuestCookieName string
	MaxConcurrent   int
	FrontendOrigin  string

	AIEnabled       bool
	Provider        string
	GeminiAPIKey    string
	Model           string
	Temperature     float64
	MaxOutputTokens int

	RequestTimeout  time.Duration
	ToolTimeout     time.Duration
	TotalTimeout    time.Duration
	MaxToolRounds   int
	MaxToolCalls    int
	MaxHistoryItems int

	ConversationTTL         time.Duration
	ConversationMax         int
	ConversationMaxMessages int

	RateLimit  int
	RateWindow time.Duration

	PricingBaseURL string
	PricingTimeout time.Duration
	ParcelBaseURL  string
	ParcelTimeout  time.Duration
}

// Load reads configuration from the process environment.
func Load() (Config, error) {
	return LoadFrom(sharedconfig.Load())
}

// LoadFrom reads configuration through the shared loader so parsing and
// validation behave consistently with the other services.
func LoadFrom(loader sharedconfig.Loader) (Config, error) {
	appEnv, err := sharedconfig.LoadAppEnv(loader)
	if err != nil {
		return Config{}, err
	}

	logLevel, err := loader.EnumFold("LOG_LEVEL", "info", "debug", "info", "warn", "warning", "error")
	if err != nil {
		return Config{}, err
	}

	port, err := loader.Port("PORT", 8087)
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

	cookieName := loader.String("COOKIE_NAME", "sd_access")
	if !cookieNamePattern.MatchString(cookieName) {
		return Config{}, errors.New("COOKIE_NAME must be a valid cookie name")
	}

	guestCookieName := loader.String("AI_GUEST_COOKIE_NAME", "sd_ai_guest")
	if !cookieNamePattern.MatchString(guestCookieName) {
		return Config{}, errors.New("AI_GUEST_COOKIE_NAME must be a valid cookie name")
	}
	if guestCookieName == cookieName {
		return Config{}, errors.New("AI_GUEST_COOKIE_NAME must differ from COOKIE_NAME")
	}

	maxConcurrent, err := loader.Int("AI_MAX_CONCURRENT", 3)
	if err != nil {
		return Config{}, err
	}
	if maxConcurrent < 1 || maxConcurrent > 20 {
		return Config{}, errors.New("AI_MAX_CONCURRENT must be between 1 and 20")
	}

	frontendOrigin, err := serviceOrigin(loader.String("FRONTEND_BASE_URL", "http://localhost:8080"), "FRONTEND_BASE_URL")
	if err != nil {
		return Config{}, err
	}

	enabled, err := loaderBool(loader, "AI_ENABLED", true)
	if err != nil {
		return Config{}, err
	}

	provider, err := loader.EnumFold("AI_PROVIDER", "gemini", "gemini", "mock")
	if err != nil {
		return Config{}, err
	}
	if provider == "mock" && appEnv != "development" {
		return Config{}, errors.New("AI_PROVIDER=mock is only allowed in development")
	}

	model := loader.String("AI_MODEL", defaultModel)
	if len(model) > 100 || strings.ContainsAny(model, " \t\r\n") {
		return Config{}, errors.New("AI_MODEL must be a single model identifier of at most 100 characters")
	}

	temperature, err := loaderFloat(loader, "AI_TEMPERATURE", 0.2)
	if err != nil {
		return Config{}, err
	}
	if math.IsNaN(temperature) || math.IsInf(temperature, 0) || temperature < 0 || temperature > 2 {
		return Config{}, errors.New("AI_TEMPERATURE must be a finite number between 0 and 2")
	}

	maxOutputTokens, err := loader.Int("AI_MAX_OUTPUT_TOKENS", 1024)
	if err != nil {
		return Config{}, err
	}
	if maxOutputTokens < 1 || maxOutputTokens > 8192 {
		return Config{}, errors.New("AI_MAX_OUTPUT_TOKENS must be between 1 and 8192")
	}

	requestTimeout, err := loader.Duration("AI_REQUEST_TIMEOUT", 15*time.Second)
	if err != nil {
		return Config{}, err
	}
	toolTimeout, err := loader.Duration("AI_TOOL_TIMEOUT", 5*time.Second)
	if err != nil {
		return Config{}, err
	}
	totalTimeout, err := loader.Duration("AI_TOTAL_TIMEOUT", 25*time.Second)
	if err != nil {
		return Config{}, err
	}
	if totalTimeout < requestTimeout {
		return Config{}, errors.New("AI_TOTAL_TIMEOUT must be at least AI_REQUEST_TIMEOUT")
	}

	maxToolRounds, err := loader.Int("AI_MAX_TOOL_ROUNDS", 3)
	if err != nil {
		return Config{}, err
	}
	if maxToolRounds < 1 || maxToolRounds > 5 {
		return Config{}, errors.New("AI_MAX_TOOL_ROUNDS must be between 1 and 5")
	}

	maxToolCalls, err := loader.Int("AI_MAX_TOOL_CALLS", 4)
	if err != nil {
		return Config{}, err
	}
	if maxToolCalls < 1 || maxToolCalls > 8 {
		return Config{}, errors.New("AI_MAX_TOOL_CALLS must be between 1 and 8")
	}

	maxHistory, err := loader.Int("AI_MAX_HISTORY_MESSAGES", 12)
	if err != nil {
		return Config{}, err
	}
	if maxHistory < 1 || maxHistory > 50 {
		return Config{}, errors.New("AI_MAX_HISTORY_MESSAGES must be between 1 and 50")
	}

	conversationTTL, err := loader.Duration("AI_CONVERSATION_TTL", 30*time.Minute)
	if err != nil {
		return Config{}, err
	}

	conversationMax, err := loader.Int("AI_CONVERSATION_MAX", 10000)
	if err != nil {
		return Config{}, err
	}
	if conversationMax < 1 || conversationMax > 100000 {
		return Config{}, errors.New("AI_CONVERSATION_MAX must be between 1 and 100000")
	}

	conversationMaxMessages, err := loader.Int("AI_CONVERSATION_MAX_MESSAGES", 40)
	if err != nil {
		return Config{}, err
	}
	if conversationMaxMessages < 1 || conversationMaxMessages > 200 {
		return Config{}, errors.New("AI_CONVERSATION_MAX_MESSAGES must be between 1 and 200")
	}
	if conversationMaxMessages < maxHistory {
		return Config{}, errors.New("AI_CONVERSATION_MAX_MESSAGES must be at least AI_MAX_HISTORY_MESSAGES")
	}

	rateLimit, err := loader.Int("AI_RATE_LIMIT", 15)
	if err != nil {
		return Config{}, err
	}
	if rateLimit < 1 || rateLimit > 1000 {
		return Config{}, errors.New("AI_RATE_LIMIT must be between 1 and 1000")
	}
	rateWindow, err := loader.Duration("AI_RATE_WINDOW", time.Minute)
	if err != nil {
		return Config{}, err
	}

	pricingURL, err := serviceOrigin(loader.String("PRICING_BASE_URL", "http://pricing:8083"), "PRICING_BASE_URL")
	if err != nil {
		return Config{}, err
	}
	pricingTimeout, err := loader.Duration("PRICING_TIMEOUT", 3*time.Second)
	if err != nil {
		return Config{}, err
	}

	parcelURL, err := serviceOrigin(loader.String("PARCEL_BASE_URL", "http://parcel:8082"), "PARCEL_BASE_URL")
	if err != nil {
		return Config{}, err
	}
	parcelTimeout, err := loader.Duration("PARCEL_TIMEOUT", 3*time.Second)
	if err != nil {
		return Config{}, err
	}

	apiKey := loader.String("GEMINI_API_KEY", "")

	return Config{
		Port:                    port,
		AppEnv:                  appEnv,
		LogLevel:                logLevel,
		TrustedProxies:          proxies,
		AccessSecret:            accessSecret,
		CookieName:              cookieName,
		GuestCookieName:         guestCookieName,
		MaxConcurrent:           maxConcurrent,
		FrontendOrigin:          frontendOrigin,
		AIEnabled:               enabled,
		Provider:                provider,
		GeminiAPIKey:            apiKey,
		Model:                   model,
		Temperature:             temperature,
		MaxOutputTokens:         maxOutputTokens,
		RequestTimeout:          requestTimeout,
		ToolTimeout:             toolTimeout,
		TotalTimeout:            totalTimeout,
		MaxToolRounds:           maxToolRounds,
		MaxToolCalls:            maxToolCalls,
		MaxHistoryItems:         maxHistory,
		ConversationTTL:         conversationTTL,
		ConversationMax:         conversationMax,
		ConversationMaxMessages: conversationMaxMessages,
		RateLimit:               rateLimit,
		RateWindow:              rateWindow,
		PricingBaseURL:          pricingURL,
		PricingTimeout:          pricingTimeout,
		ParcelBaseURL:           parcelURL,
		ParcelTimeout:           parcelTimeout,
	}, nil
}

// Addr returns the listen address.
func (c Config) Addr() string {
	return ":" + strconv.Itoa(c.Port)
}

func loaderBool(loader sharedconfig.Loader, key string, fallback bool) (bool, error) {
	raw := loader.String(key, "")
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("%s must be true or false", key)
	}
	return value, nil
}

func loaderFloat(loader sharedconfig.Loader, key string, fallback float64) (float64, error) {
	raw := loader.String(key, "")
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, fmt.Errorf("%s must be a number", key)
	}
	return value, nil
}

func serviceOrigin(raw, key string) (string, error) {
	trimmed := strings.TrimRight(strings.TrimSpace(raw), "/")
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Hostname() == "" ||
		(parsed.Scheme != "http" && parsed.Scheme != "https") ||
		parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" ||
		(parsed.Path != "" && parsed.Path != "/") {
		return "", fmt.Errorf("%s must be an HTTP(S) service origin", key)
	}
	return trimmed, nil
}
