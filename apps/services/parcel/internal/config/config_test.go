package config

import (
	"strings"
	"testing"

	sharedconfig "github.com/Vineth-R/swiftdrop/packages/go-common/config"
)

func TestParcelRelayConfiguration(t *testing.T) {
	for _, tc := range []struct {
		name    string
		url     string
		wantErr bool
	}{
		{name: "disabled"},
		{name: "configured", url: "amqp://rabbitmq:5672/"},
		{name: "wrong scheme", url: "https://local:PRIVATE_RABBIT_PASSWORD@rabbitmq:5672/", wantErr: true},
		{name: "missing host", url: "amqp:///missing-host", wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			loader := sharedconfig.NewLoader(func(key string) (string, bool) {
				values := map[string]string{
					"APP_ENV":           "development",
					"RABBITMQ_URL":      tc.url,
					"RABBITMQ_EXCHANGE": "custom.parcel.events",
				}
				value, ok := values[key]
				return value, ok
			})
			cfg, err := LoadFrom(loader)
			if tc.wantErr {
				if err == nil {
					t.Fatal("invalid relay URL accepted")
				}
				if strings.Contains(err.Error(), tc.url) || strings.Contains(err.Error(), "PRIVATE_RABBIT_PASSWORD") {
					t.Fatal("relay configuration error reflected credentials")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if cfg.RabbitMQURL != tc.url || cfg.RabbitMQExchange != "custom.parcel.events" {
				t.Fatal("configured relay origin or exchange was not retained")
			}
		})
	}
}

func TestParcelDependencyConfiguration(t *testing.T) {
	for _, tc := range []struct {
		name, key, value string
		fail             bool
	}{{"pricing credentials", "PRICING_BASE_URL", "http://user:secret@pricing:8083", true}, {"pricing scheme", "PRICING_BASE_URL", "file:///pricing", true}, {"pricing path", "PRICING_BASE_URL", "http://pricing:8083/path", true}, {"pricing timeout", "PRICING_TIMEOUT", "0s", true}, {"identity credentials", "IDENTITY_BASE_URL", "http://user:secret@identity:8081", true}, {"identity timeout", "IDENTITY_TIMEOUT", "-1s", true}, {"pricing valid", "PRICING_BASE_URL", "http://127.0.0.1:8083/", false}, {"identity valid", "IDENTITY_BASE_URL", "https://identity.example", false}, {"development short token", "INTERNAL_TOKEN", "tiny", false}} {
		t.Run(tc.name, func(t *testing.T) {
			loader := sharedconfig.NewLoader(func(key string) (string, bool) {
				if key == "APP_ENV" {
					return "development", true
				}
				if key == tc.key {
					return tc.value, true
				}
				return "", false
			})
			cfg, err := LoadFrom(loader)
			if tc.fail {
				if err == nil {
					t.Fatal("invalid configuration accepted")
				}
				if strings.Contains(err.Error(), "secret@") {
					t.Fatal("configuration leaked credentials")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if tc.key == "INTERNAL_TOKEN" && (cfg.InternalToken != "" || cfg.InternalTokenProblem == "") {
				t.Fatal("weak development token did not fail closed")
			}
		})
	}
	loader := sharedconfig.NewLoader(func(key string) (string, bool) {
		if key == "APP_ENV" {
			return "production", true
		}
		if key == "INTERNAL_TOKEN" {
			return "short", true
		}
		return "", false
	})
	if _, err := LoadFrom(loader); err == nil {
		t.Fatal("weak production internal token accepted")
	}
}

func TestLoadFromDefaults(t *testing.T) {
	loader := sharedconfig.NewLoader(func(key string) (string, bool) {
		if key == "APP_ENV" {
			return "development", true
		}
		return "", false
	})

	config, err := LoadFrom(loader)
	if err != nil {
		t.Fatalf("LoadFrom() returned error: %v", err)
	}

	if config.Port != 8082 {
		t.Fatalf("Port = %d, want 8082", config.Port)
	}
	if config.AppEnv != "development" {
		t.Fatalf("AppEnv = %q, want development", config.AppEnv)
	}
	if config.LogLevel != "info" {
		t.Fatalf("LogLevel = %q, want info", config.LogLevel)
	}
	if config.Addr() != ":8082" {
		t.Fatalf("Addr() = %q, want :8082", config.Addr())
	}
	if config.AccessSecret != "" {
		t.Fatalf("AccessSecret = %q, want empty", config.AccessSecret)
	}
	if config.CookieName != "sd_access" {
		t.Fatalf("CookieName = %q, want sd_access", config.CookieName)
	}
	if config.ProofStorageDir != "" {
		t.Fatalf("ProofStorageDir = %q, want empty", config.ProofStorageDir)
	}
}

func TestLoadFromRetainsProofStorageDirectory(t *testing.T) {
	loader := sharedconfig.NewLoader(func(key string) (string, bool) {
		values := map[string]string{
			"APP_ENV":           "development",
			"PROOF_STORAGE_DIR": `C:\private\swiftdrop-proofs`,
		}
		value, ok := values[key]
		return value, ok
	})

	config, err := LoadFrom(loader)
	if err != nil {
		t.Fatal(err)
	}
	if config.ProofStorageDir != `C:\private\swiftdrop-proofs` {
		t.Fatalf("ProofStorageDir = %q", config.ProofStorageDir)
	}
}

func TestLoadFromRequiresExplicitAppEnv(t *testing.T) {
	tests := []struct {
		name    string
		appEnv  string
		present bool
		want    string
		wantErr bool
	}{
		{name: "unset", wantErr: true},
		{name: "empty", appEnv: "", present: true, wantErr: true},
		{name: "whitespace", appEnv: "   ", present: true, wantErr: true},
		{name: "unknown", appEnv: "local", present: true, wantErr: true},
		{name: "development", appEnv: "development", present: true, want: "development"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			loader := sharedconfig.NewLoader(func(key string) (string, bool) {
				if key == "APP_ENV" {
					return test.appEnv, test.present
				}
				return "", false
			})

			config, err := LoadFrom(loader)
			if test.wantErr {
				if err == nil {
					t.Fatalf("LoadFrom() accepted APP_ENV=%q", test.appEnv)
				}
				return
			}
			if err != nil {
				t.Fatalf("LoadFrom() returned error: %v", err)
			}
			if config.AppEnv != test.want {
				t.Fatalf("AppEnv = %q, want %q", config.AppEnv, test.want)
			}
		})
	}
}

func TestLoadFromValidatesAccessSecretOutsideDevelopment(t *testing.T) {
	weak := strings.Repeat("w", sharedconfig.MinAccessSecretBytes-1)
	valid := strings.Repeat("v", sharedconfig.MinAccessSecretBytes)
	placeholder := "change-me-access-secret-use-32-or-more-random-characters"

	tests := []struct {
		name    string
		appEnv  string
		secret  string
		present bool
		want    string
		wantErr bool
	}{
		{name: "development weak", appEnv: "development", secret: weak, present: true, want: weak},
		{name: "production weak", appEnv: "production", secret: weak, present: true, wantErr: true},
		{name: "production placeholder", appEnv: "production", secret: placeholder, present: true, wantErr: true},
		{name: "production valid", appEnv: "production", secret: valid, present: true, want: valid},
		{name: "production empty", appEnv: "production", secret: "", present: true, want: ""},
		{name: "production unset", appEnv: "production", want: ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			loader := sharedconfig.NewLoader(func(key string) (string, bool) {
				if key == "APP_ENV" {
					return test.appEnv, true
				}
				if key == "JWT_ACCESS_SECRET" {
					return test.secret, test.present
				}
				return "", false
			})

			config, err := LoadFrom(loader)
			if test.wantErr {
				if err == nil {
					t.Fatal("LoadFrom() should have failed")
				}
				if test.secret != "" && strings.Contains(err.Error(), test.secret) {
					t.Fatal("the error text contains the secret")
				}
				return
			}
			if err != nil {
				t.Fatalf("LoadFrom() returned error: %v", err)
			}
			if config.AccessSecret != test.want {
				t.Fatalf("AccessSecret = %q, want %q", config.AccessSecret, test.want)
			}
		})
	}
}

func TestLoadFromIsCaseInsensitiveAndTrims(t *testing.T) {
	secret := strings.Repeat("s", sharedconfig.MinAccessSecretBytes)

	loader := sharedconfig.NewLoader(func(key string) (string, bool) {
		values := map[string]string{
			"APP_ENV":           " PRODUCTION ",
			"LOG_LEVEL":         "WARNING",
			"PORT":              " 9092 ",
			"DATABASE_URL":      " postgres://example ",
			"JWT_ACCESS_SECRET": " " + secret + " ",
			"COOKIE_NAME":       " sd_ops ",
		}
		value, ok := values[key]
		return value, ok
	})

	config, err := LoadFrom(loader)
	if err != nil {
		t.Fatalf("LoadFrom() returned error: %v", err)
	}

	if config.AppEnv != "production" {
		t.Fatalf("AppEnv = %q, want production", config.AppEnv)
	}
	if config.LogLevel != "warning" {
		t.Fatalf("LogLevel = %q, want warning", config.LogLevel)
	}
	if config.Port != 9092 {
		t.Fatalf("Port = %d, want 9092", config.Port)
	}
	if config.DatabaseURL != "postgres://example" {
		t.Fatalf("DatabaseURL = %q", config.DatabaseURL)
	}
	if config.AccessSecret != secret {
		t.Fatalf("AccessSecret = %q, want %q", config.AccessSecret, secret)
	}
	if config.CookieName != "sd_ops" {
		t.Fatalf("CookieName = %q, want sd_ops", config.CookieName)
	}
}

func TestLoadFromRejectsInvalidConfig(t *testing.T) {
	loader := sharedconfig.NewLoader(func(key string) (string, bool) {
		if key == "APP_ENV" {
			return "local", true
		}
		return "", false
	})

	if _, err := LoadFrom(loader); err == nil {
		t.Fatal("LoadFrom() should fail for an unsupported APP_ENV")
	}
}
