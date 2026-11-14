package config

import (
	"strings"
	"testing"

	sharedconfig "github.com/Vineth-R/swiftdrop/packages/go-common/config"
)

func webhookLoader(values map[string]string) sharedconfig.Loader {
	return sharedconfig.NewLoader(func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	})
}

func TestLoadWebhookSecretKey(t *testing.T) {
	valid := strings.Repeat("1f", 32)
	tests := []struct {
		name    string
		value   string
		set     bool
		want    string
		wantErr bool
	}{
		{name: "missing", set: false, want: ""},
		{name: "empty", value: "", set: true, want: ""},
		{name: "whitespace", value: "   ", set: true, want: ""},
		{name: "valid 32-byte key", value: valid, set: true, want: valid},
		{name: "valid uppercase key", value: strings.ToUpper(valid), set: true, want: strings.ToUpper(valid)},
		{name: "invalid hex", value: strings.Repeat("zz", 32), set: true, wantErr: true},
		{name: "wrong length short", value: strings.Repeat("ab", 16), set: true, wantErr: true},
		{name: "wrong length long", value: strings.Repeat("ab", 33), set: true, wantErr: true},
		{name: "all-zero placeholder", value: strings.Repeat("0", 64), set: true, wantErr: true},
		{name: "change-me placeholder", value: "change-me-change-me-change-me-change-me-change-me-change-me-0000", set: true, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			values := map[string]string{}
			if test.set {
				values["WEBHOOK_SECRET_KEY"] = test.value
			}
			got, err := LoadWebhookSecretKey(webhookLoader(values))
			if test.wantErr {
				if err == nil {
					t.Fatalf("LoadWebhookSecretKey() accepted %q", test.value)
				}
				if strings.Contains(err.Error(), test.value) && test.value != "" {
					t.Fatalf("error leaks the key: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("LoadWebhookSecretKey() returned error: %v", err)
			}
			if got != test.want {
				t.Fatalf("LoadWebhookSecretKey() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestLoadFromAppliesWebhookKeyInEveryEnvironment(t *testing.T) {
	valid := strings.Repeat("2a", 32)
	for _, appEnv := range []string{"development", "staging", "production"} {
		t.Run(appEnv+" valid key", func(t *testing.T) {
			config, err := LoadFrom(webhookLoader(map[string]string{
				"APP_ENV":            appEnv,
				"WEBHOOK_SECRET_KEY": valid,
			}))
			if err != nil {
				t.Fatalf("LoadFrom() returned error: %v", err)
			}
			if config.WebhookSecretKey != valid {
				t.Fatalf("WebhookSecretKey = %q", config.WebhookSecretKey)
			}
		})

		t.Run(appEnv+" missing key is disabled", func(t *testing.T) {
			config, err := LoadFrom(webhookLoader(map[string]string{"APP_ENV": appEnv}))
			if err != nil {
				t.Fatalf("LoadFrom() returned error: %v", err)
			}
			if config.WebhookSecretKey != "" {
				t.Fatalf("WebhookSecretKey = %q, want empty", config.WebhookSecretKey)
			}
		})

		t.Run(appEnv+" invalid key is rejected", func(t *testing.T) {
			if _, err := LoadFrom(webhookLoader(map[string]string{
				"APP_ENV":            appEnv,
				"WEBHOOK_SECRET_KEY": "not-a-key",
			})); err == nil {
				t.Fatal("LoadFrom() accepted an invalid key")
			}
		})

		t.Run(appEnv+" placeholder is rejected", func(t *testing.T) {
			if _, err := LoadFrom(webhookLoader(map[string]string{
				"APP_ENV":            appEnv,
				"WEBHOOK_SECRET_KEY": strings.Repeat("0", 64),
			})); err == nil {
				t.Fatal("LoadFrom() accepted a placeholder key")
			}
		})
	}
}

func TestLoadFromWebhookAuthDefaults(t *testing.T) {
	config, err := LoadFrom(webhookLoader(map[string]string{"APP_ENV": "development"}))
	if err != nil {
		t.Fatalf("LoadFrom() returned error: %v", err)
	}
	if config.CookieName != "sd_access" {
		t.Fatalf("CookieName = %q, want sd_access", config.CookieName)
	}
	if config.AccessSecret != "" {
		t.Fatalf("AccessSecret = %q, want empty", config.AccessSecret)
	}
}

func TestLoadWebhookKeyring(t *testing.T) {
	active := strings.Repeat("1f", 32)
	previous := strings.Repeat("2e", 32)

	id, key, keys, err := LoadWebhookKeyring(webhookLoader(map[string]string{
		"WEBHOOK_SECRET_KEY":    active,
		"WEBHOOK_SECRET_KEY_ID": "k2",
		"WEBHOOK_SECRET_KEYS":   "k1=" + previous + ", k0=" + strings.Repeat("3d", 32),
	}))
	if err != nil {
		t.Fatalf("LoadWebhookKeyring() error = %v", err)
	}
	if id != "k2" || key != active {
		t.Fatalf("active = %q/%q", id, key)
	}
	if keys["k1"] != previous || len(keys) != 2 {
		t.Fatalf("previous keys = %v", keys)
	}
}

func TestLoadWebhookKeyringDefaultsActiveID(t *testing.T) {
	active := strings.Repeat("1f", 32)
	id, _, keys, err := LoadWebhookKeyring(webhookLoader(map[string]string{"WEBHOOK_SECRET_KEY": active}))
	if err != nil {
		t.Fatalf("LoadWebhookKeyring() error = %v", err)
	}
	if id != "k1" || len(keys) != 0 {
		t.Fatalf("id = %q keys = %v", id, keys)
	}
}

func TestLoadWebhookKeyringRejectsMalformedPreviousKeys(t *testing.T) {
	active := strings.Repeat("1f", 32)
	for _, value := range []string{
		"k1",                            // no =
		"bad id=" + active,              // invalid id
		"k1=not-hex",                    // invalid key
		"k1=" + strings.Repeat("0", 64), // placeholder
		"k1=" + active,                  // repeats the active id
	} {
		if _, _, _, err := LoadWebhookKeyring(webhookLoader(map[string]string{
			"WEBHOOK_SECRET_KEY":    active,
			"WEBHOOK_SECRET_KEY_ID": "k1",
			"WEBHOOK_SECRET_KEYS":   value,
		})); err == nil {
			t.Fatalf("LoadWebhookKeyring() accepted %q", value)
		}
	}
}
