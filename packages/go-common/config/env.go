package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Loader struct {
	lookup func(string) (string, bool)
}

func Load() Loader {
	return Loader{lookup: os.LookupEnv}
}

func NewLoader(lookup func(string) (string, bool)) Loader {
	return Loader{lookup: lookup}
}

func (l Loader) String(key, fallback string) string {
	if value, ok := l.lookup(key); ok && value != "" {
		return value
	}
	return fallback
}

func (l Loader) Required(key string) (string, error) {
	value, ok := l.lookup(key)
	if !ok || strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("missing required environment variable %s", key)
	}
	return value, nil
}

func (l Loader) Int(key string, fallback int) (int, error) {
	raw := l.String(key, "")
	if raw == "" {
		return fallback, nil
	}

	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer: %w", key, err)
	}
	return value, nil
}

func (l Loader) Port(key string, fallback int) (int, error) {
	value, err := l.Int(key, fallback)
	if err != nil {
		return 0, err
	}
	if value < 1 || value > 65535 {
		return 0, fmt.Errorf("%s must be between 1 and 65535", key)
	}
	return value, nil
}

func (l Loader) Duration(key string, fallback time.Duration) (time.Duration, error) {
	raw := l.String(key, "")
	if raw == "" {
		return fallback, nil
	}

	value, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("%s must be a duration such as 30s or 1m: %w", key, err)
	}
	if value <= 0 {
		return 0, fmt.Errorf("%s must be greater than zero", key)
	}
	return value, nil
}

func (l Loader) Enum(key, fallback string, allowed ...string) (string, error) {
	value := l.String(key, fallback)
	for _, candidate := range allowed {
		if value == candidate {
			return value, nil
		}
	}
	return "", fmt.Errorf("%s must be one of %s", key, strings.Join(allowed, ", "))
}
