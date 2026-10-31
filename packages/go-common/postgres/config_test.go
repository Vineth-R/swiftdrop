package postgres

import (
	"errors"
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

func TestLoadConfigUsesDatabaseURL(t *testing.T) {
	config, err := LoadConfig(loader(map[string]string{
		"DATABASE_URL": "postgres://user:secret@db:5432/swiftdrop?sslmode=disable",
	}))
	if err != nil {
		t.Fatalf("LoadConfig() error: %v", err)
	}
	if !config.Configured() {
		t.Fatal("expected configured database")
	}
	if config.URL != "postgres://user:secret@db:5432/swiftdrop?sslmode=disable" {
		t.Fatalf("unexpected URL %q", config.URL)
	}
	if config.MaxConns != DefaultMaxConns || config.RetryAttempts != DefaultRetryAttempts {
		t.Fatalf("unexpected defaults: %+v", config)
	}
}

func TestLoadConfigBuildsURLFromParts(t *testing.T) {
	config, err := LoadConfig(loader(map[string]string{
		"POSTGRES_HOST":     "postgres",
		"POSTGRES_PORT":     "5544",
		"POSTGRES_USER":     "swiftdrop",
		"POSTGRES_PASSWORD": "p@ss word",
		"POSTGRES_DB":       "swiftdrop",
		"POSTGRES_SSLMODE":  "require",
	}))
	if err != nil {
		t.Fatalf("LoadConfig() error: %v", err)
	}
	if !strings.HasPrefix(config.URL, "postgres://swiftdrop:p%40ss%20word@postgres:5544/swiftdrop") {
		t.Fatalf("unexpected URL %q", config.URL)
	}
	if !strings.Contains(config.URL, "sslmode=require") {
		t.Fatalf("missing sslmode in %q", config.URL)
	}
}

func TestLoadConfigUnconfigured(t *testing.T) {
	config, err := LoadConfig(loader(map[string]string{}))
	if err != nil {
		t.Fatalf("LoadConfig() error: %v", err)
	}
	if config.Configured() {
		t.Fatal("expected unconfigured database")
	}
}

func TestLoadConfigValidationErrorsDoNotLeakURL(t *testing.T) {
	cases := map[string]string{
		"scheme": "mysql://user:secret@db:3306/swiftdrop",
		"host":   "postgres://user:secret@/swiftdrop",
		"port":   "postgres://user:secret@db:notaport/swiftdrop",
	}

	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := LoadConfig(loader(map[string]string{"DATABASE_URL": raw}))
			if err == nil {
				t.Fatal("expected validation error")
			}
			if !errors.Is(err, ErrInvalidConfig) {
				t.Fatalf("expected ErrInvalidConfig, got %v", err)
			}
			if strings.Contains(err.Error(), "secret") {
				t.Fatalf("validation error leaked credentials: %v", err)
			}
		})
	}
}

func TestLoadConfigRejectsMissingParts(t *testing.T) {
	_, err := LoadConfig(loader(map[string]string{"POSTGRES_HOST": "postgres"}))
	if err == nil {
		t.Fatal("expected error for missing user/database")
	}
}

func TestNextDelay(t *testing.T) {
	if got := nextDelay(250*time.Millisecond, 5*time.Second); got != 500*time.Millisecond {
		t.Fatalf("nextDelay() = %v", got)
	}
	if got := nextDelay(4*time.Second, 5*time.Second); got != 5*time.Second {
		t.Fatalf("nextDelay() should cap at maximum, got %v", got)
	}
	if got := nextDelay(0, 5*time.Second); got != DefaultRetryMin {
		t.Fatalf("nextDelay(0) = %v", got)
	}
}
