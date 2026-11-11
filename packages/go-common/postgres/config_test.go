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

func TestLoadConfigReadsPoolSettings(t *testing.T) {
	config, err := LoadConfig(loader(map[string]string{
		"DATABASE_URL":           "postgres://user:secret@db:5432/swiftdrop?sslmode=disable",
		"DB_MAX_OPEN_CONNS":      "15",
		"DB_MAX_IDLE_CONNS":      "0",
		"DB_CONN_MAX_LIFETIME":   "15m",
		"DB_CONN_MAX_IDLE_TIME":  "2m",
		"DB_CONNECT_TIMEOUT":     "3s",
		"DB_HEALTH_CHECK_PERIOD": "30s",
	}))
	if err != nil {
		t.Fatalf("LoadConfig() error: %v", err)
	}
	if config.MaxConns != 15 || config.MinConns != 0 {
		t.Fatalf("pool = %d/%d", config.MaxConns, config.MinConns)
	}
	if config.MaxConnLifetime != 15*time.Minute || config.MaxConnIdleTime != 2*time.Minute {
		t.Fatalf("lifetimes = %v/%v", config.MaxConnLifetime, config.MaxConnIdleTime)
	}
	if config.ConnectTimeout != 3*time.Second || config.HealthCheckPeriod != 30*time.Second {
		t.Fatalf("timeouts = %v/%v", config.ConnectTimeout, config.HealthCheckPeriod)
	}
}

func TestLoadConfigKeepsLegacyPoolAliases(t *testing.T) {
	config, err := LoadConfig(loader(map[string]string{
		"DATABASE_URL":       "postgres://user:secret@db:5432/swiftdrop?sslmode=disable",
		"POSTGRES_MAX_CONNS": "7",
		"POSTGRES_MIN_CONNS": "1",
		"DB_MAX_OPEN_CONNS":  "12",
	}))
	if err != nil {
		t.Fatalf("LoadConfig() error: %v", err)
	}
	if config.MaxConns != 12 || config.MinConns != 1 {
		t.Fatalf("pool = %d/%d", config.MaxConns, config.MinConns)
	}
}

func TestLoadConfigFromURLUsesExplicitURL(t *testing.T) {
	config, err := LoadConfigFromURL("postgres://user:secret@db:5432/swiftdrop?sslmode=disable")
	if err != nil {
		t.Fatalf("LoadConfigFromURL() error: %v", err)
	}
	if config.URL != "postgres://user:secret@db:5432/swiftdrop?sslmode=disable" {
		t.Fatalf("unexpected URL %q", config.URL)
	}
}

func TestLoadConfigRequiresSSLForNeonHosts(t *testing.T) {
	cases := map[string]string{
		"missing": "postgres://user:secret@ep-test-pooler.us-east-2.aws.neon.tech/neondb",
		"disable": "postgres://user:secret@ep-test-pooler.us-east-2.aws.neon.tech/neondb?sslmode=disable",
		"prefer":  "postgres://user:secret@ep-test-pooler.us-east-2.aws.neon.tech/neondb?sslmode=prefer",
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := LoadConfig(loader(map[string]string{"DATABASE_URL": raw}))
			if !errors.Is(err, ErrInvalidConfig) {
				t.Fatalf("expected ErrInvalidConfig, got %v", err)
			}
			if strings.Contains(err.Error(), "secret") {
				t.Fatalf("validation error leaked credentials: %v", err)
			}
		})
	}

	config, err := LoadConfig(loader(map[string]string{
		"DATABASE_URL": "postgres://user:secret@ep-test-pooler.us-east-2.aws.neon.tech/neondb?sslmode=require",
	}))
	if err != nil || !config.Configured() {
		t.Fatalf("Neon URL with sslmode=require rejected: %v", err)
	}
}

func TestIsNeonHost(t *testing.T) {
	for host, want := range map[string]bool{
		"ep-test-pooler.us-east-2.aws.neon.tech": true,
		"EP-TEST.NEON.TECH":                      true,
		"postgres":                               false,
		"neon.tech.example.com":                  false,
	} {
		if got := IsNeonHost(host); got != want {
			t.Fatalf("IsNeonHost(%q) = %v, want %v", host, got, want)
		}
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
