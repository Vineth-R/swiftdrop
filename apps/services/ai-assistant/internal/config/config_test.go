package config

import (
	"strings"
	"testing"
	"time"

	sharedconfig "github.com/Vineth-R/swiftdrop/packages/go-common/config"
)

func loader(values map[string]string) sharedconfig.Loader {
	return sharedconfig.NewLoader(func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	})
}

func baseEnv() map[string]string {
	return map[string]string{
		"APP_ENV": "development",
	}
}

func TestLoadDefaults(t *testing.T) {
	cfg, err := LoadFrom(loader(baseEnv()))
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}

	if cfg.Port != 8087 {
		t.Errorf("Port = %d, want 8087", cfg.Port)
	}
	if cfg.Provider != "gemini" {
		t.Errorf("Provider = %q, want gemini", cfg.Provider)
	}
	if cfg.Model != "gemini-3.5-flash" {
		t.Errorf("Model = %q, want gemini-3.5-flash", cfg.Model)
	}
	if !cfg.AIEnabled {
		t.Error("AIEnabled = false, want true")
	}
	if cfg.Temperature != 0.2 {
		t.Errorf("Temperature = %v, want 0.2", cfg.Temperature)
	}
	if cfg.MaxToolRounds != 3 || cfg.MaxToolCalls != 4 {
		t.Errorf("tool limits = %d/%d, want 3/4", cfg.MaxToolRounds, cfg.MaxToolCalls)
	}
	if cfg.RateLimit != 15 || cfg.RateWindow != time.Minute {
		t.Errorf("rate limit = %d/%v, want 15/1m", cfg.RateLimit, cfg.RateWindow)
	}
	if cfg.ConversationTTL != 30*time.Minute {
		t.Errorf("ConversationTTL = %v, want 30m", cfg.ConversationTTL)
	}
}

func TestLoadRejectsMissingAppEnv(t *testing.T) {
	_, err := LoadFrom(loader(map[string]string{}))
	if err == nil || !strings.Contains(err.Error(), "APP_ENV") {
		t.Fatalf("expected APP_ENV error, got %v", err)
	}
}

func TestLoadRejectsBadValues(t *testing.T) {
	cases := map[string]map[string]string{
		"temperature":     {"APP_ENV": "development", "AI_TEMPERATURE": "3"},
		"temperature NaN": {"APP_ENV": "development", "AI_TEMPERATURE": "NaN"},
		"provider":        {"APP_ENV": "development", "AI_PROVIDER": "openai"},
		"max tokens":      {"APP_ENV": "development", "AI_MAX_OUTPUT_TOKENS": "0"},
		"tool rounds":     {"APP_ENV": "development", "AI_MAX_TOOL_ROUNDS": "9"},
		"tool calls":      {"APP_ENV": "development", "AI_MAX_TOOL_CALLS": "0"},
		"total timeout":   {"APP_ENV": "development", "AI_REQUEST_TIMEOUT": "10s", "AI_TOTAL_TIMEOUT": "5s"},
		"history":         {"APP_ENV": "development", "AI_MAX_HISTORY_MESSAGES": "0"},
		"history max":     {"APP_ENV": "development", "AI_MAX_HISTORY_MESSAGES": "20", "AI_CONVERSATION_MAX_MESSAGES": "10"},
		"rate limit":      {"APP_ENV": "development", "AI_RATE_LIMIT": "0"},
		"pricing URL":     {"APP_ENV": "development", "PRICING_BASE_URL": "ftp://pricing"},
		"parcel path":     {"APP_ENV": "development", "PARCEL_BASE_URL": "http://parcel:8082/api"},
		"bool":            {"APP_ENV": "development", "AI_ENABLED": "sometimes"},
		"model spaces":    {"APP_ENV": "development", "AI_MODEL": "gemini 2.5"},
		"internal path":   {"APP_ENV": "development", "PRICING_BASE_URL": "http://user:pass@pricing:8083"},
	}

	for name, values := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := LoadFrom(loader(values)); err == nil {
				t.Fatal("expected configuration error")
			}
		})
	}
}

func TestLoadAllowsMissingKeyOutsideDevelopment(t *testing.T) {
	cfg, err := LoadFrom(loader(map[string]string{"APP_ENV": "production"}))
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	if cfg.GeminiAPIKey != "" {
		t.Fatalf("GeminiAPIKey = %q, want empty", cfg.GeminiAPIKey)
	}

	cfg, err = LoadFrom(loader(map[string]string{"APP_ENV": "production", "GEMINI_API_KEY": "not-a-real-key"}))
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	if cfg.GeminiAPIKey != "not-a-real-key" {
		t.Fatalf("GeminiAPIKey was not loaded")
	}
}

func TestLoadDevelopmentWithoutKeyIsAllowed(t *testing.T) {
	cfg, err := LoadFrom(loader(baseEnv()))
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	if cfg.GeminiAPIKey != "" {
		t.Fatalf("GeminiAPIKey = %q, want empty", cfg.GeminiAPIKey)
	}
}

func TestLoadMockProviderOnlyInDevelopment(t *testing.T) {
	cfg, err := LoadFrom(loader(map[string]string{"APP_ENV": "development", "AI_PROVIDER": "mock"}))
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	if cfg.Provider != "mock" {
		t.Fatalf("Provider = %q, want mock", cfg.Provider)
	}

	if _, err := LoadFrom(loader(map[string]string{"APP_ENV": "production", "AI_PROVIDER": "mock"})); err == nil {
		t.Fatal("expected mock provider to be rejected outside development")
	}
}
