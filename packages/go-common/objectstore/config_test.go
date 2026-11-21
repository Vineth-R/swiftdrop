package objectstore

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

func TestLoadConfigDisabledByDefault(t *testing.T) {
	config, err := LoadConfig(loader(map[string]string{}))
	if err != nil {
		t.Fatal(err)
	}
	if config.Enabled() {
		t.Fatal("expected disabled storage")
	}
}

func TestLoadConfigFallsBackToLocalDirectory(t *testing.T) {
	config, err := LoadConfig(loader(map[string]string{"PROOF_STORAGE_DIR": "/tmp/proofs"}))
	if err != nil {
		t.Fatal(err)
	}
	if config.Provider != ProviderLocal || config.LocalDirectory != "/tmp/proofs" {
		t.Fatalf("config = %+v", config)
	}
	if config.PresignTTL != DefaultPresignTTL {
		t.Fatalf("ttl = %v", config.PresignTTL)
	}
}

func TestLoadConfigNeonRequiresPathStyleAndCredentials(t *testing.T) {
	base := map[string]string{
		"STORAGE_PROVIDER":          "neon",
		"STORAGE_ENDPOINT":          "https://branch.storage.c-9.us-east-2.aws.neon.tech",
		"STORAGE_REGION":            "us-east-2",
		"STORAGE_BUCKET":            "delivery-proofs",
		"STORAGE_ACCESS_KEY_ID":     "nak_live_test",
		"STORAGE_SECRET_ACCESS_KEY": "nsk_live_test",
		"STORAGE_USE_PATH_STYLE":    "true",
	}

	config, err := LoadConfig(loader(base))
	if err != nil {
		t.Fatal(err)
	}
	if config.Provider != ProviderNeon || !config.UsePathStyle {
		t.Fatalf("config = %+v", config)
	}

	for name, mutate := range map[string]func(map[string]string){
		"path style false": func(v map[string]string) { v["STORAGE_USE_PATH_STYLE"] = "false" },
		"missing key":      func(v map[string]string) { delete(v, "STORAGE_ACCESS_KEY_ID") },
		"bad bucket":       func(v map[string]string) { v["STORAGE_BUCKET"] = "Bad_Bucket" },
		"bad endpoint":     func(v map[string]string) { v["STORAGE_ENDPOINT"] = "ftp://example.com" },
	} {
		t.Run(name, func(t *testing.T) {
			values := make(map[string]string, len(base))
			for key, value := range base {
				values[key] = value
			}
			mutate(values)
			_, err := LoadConfig(loader(values))
			if !errors.Is(err, ErrInvalidConfig) {
				t.Fatalf("expected ErrInvalidConfig, got %v", err)
			}
		})
	}
}

func TestLoadConfigRejectsInvalidTTL(t *testing.T) {
	for _, value := range []string{"0", "29", "3601", "abc"} {
		_, err := LoadConfig(loader(map[string]string{
			"PROOF_STORAGE_DIR":               "/tmp/proofs",
			"PROOF_PRESIGNED_URL_TTL_SECONDS": value,
		}))
		if err == nil {
			t.Fatalf("ttl %q accepted", value)
		}
	}
}

func TestLoadConfigNeverEchoesSecrets(t *testing.T) {
	_, err := LoadConfig(loader(map[string]string{
		"STORAGE_PROVIDER":          "neon",
		"STORAGE_ENDPOINT":          "not-a-url-secret",
		"STORAGE_SECRET_ACCESS_KEY": "nsk_live_super_secret",
	}))
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "nsk_live_super_secret") {
		t.Fatalf("error leaked secret: %v", err)
	}
}

func TestValidKey(t *testing.T) {
	valid := []string{
		"091d1c45-f200-4e53-8aef-afcfd3cb421d/12e5e098-362a-474e-b86a-c4f081bea0db.jpg",
		"091d1c45-f200-4e53-8aef-afcfd3cb421d/12e5e098-362a-474e-b86a-c4f081bea0db.png",
		"00000000000000000000000000000000.png",
	}
	for _, key := range valid {
		if !ValidKey(key) {
			t.Fatalf("rejected %q", key)
		}
	}
	invalid := []string{
		"", "../x.jpg", "a/b/c.jpg", "091d1c45-f200-4e53-8aef-afcfd3cb421d/x.jpg",
		"091d1c45-f200-4e53-8aef-afcfd3cb421d/12e5e098-362a-474e-b86a-c4f081bea0db.gif",
		`091d1c45-f200-4e53-8aef-afcfd3cb421d\12e5e098-362a-474e-b86a-c4f081bea0db.jpg`,
		strings.Repeat("a", 300) + ".jpg",
	}
	for _, key := range invalid {
		if ValidKey(key) {
			t.Fatalf("accepted %q", key)
		}
	}
}

func TestPresignTTLBounds(t *testing.T) {
	if MinPresignTTL >= MaxPresignTTL {
		t.Fatalf("ttl bounds = %v/%v", MinPresignTTL, MaxPresignTTL)
	}
	config, err := LoadConfig(loader(map[string]string{
		"PROOF_STORAGE_DIR":               "/tmp/proofs",
		"PROOF_PRESIGNED_URL_TTL_SECONDS": "120",
	}))
	if err != nil || config.PresignTTL != 2*time.Minute {
		t.Fatalf("ttl = %v, error = %v", config.PresignTTL, err)
	}
}
