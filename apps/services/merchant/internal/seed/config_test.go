package seed

import (
	"strings"
	"testing"

	sharedconfig "github.com/Vineth-R/swiftdrop/packages/go-common/config"
)

const testInternalToken = "test-internal-token-value"

func validValues() map[string]string {
	return map[string]string{
		"APP_ENV":                        "development",
		"DATABASE_URL":                   "postgres://swiftdrop:swiftdrop@localhost:5432/swiftdrop?sslmode=disable",
		"IDENTITY_BASE_URL":              "http://localhost:8081",
		"IDENTITY_TIMEOUT":               "3s",
		"INTERNAL_TOKEN":                 testInternalToken,
		"SEED_MERCHANT_EMAIL":            "Seed.Merchant@Example.com",
		"SEED_MERCHANT_PASSWORD":         "SeedPass123",
		"SEED_MERCHANT_FULL_NAME":        "Seed Merchant Owner",
		"SEED_MERCHANT_PHONE":            "+94771234567",
		"SEED_MERCHANT_BUSINESS_NAME":    "Seed Merchant",
		"SEED_MERCHANT_BUSINESS_REG_NO":  "pv 123456",
		"SEED_MERCHANT_CONTACT_EMAIL":    "seed.contact@example.com",
		"SEED_MERCHANT_CONTACT_PHONE":    "+94771234568",
		"SEED_MERCHANT_PICKUP_ADDRESS":   "1 Seed Street, Colombo",
		"SEED_MERCHANT_PICKUP_ZONE_CODE": "cmb",
	}
}

func newLoader(values map[string]string) sharedconfig.Loader {
	return sharedconfig.NewLoader(func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	})
}

func TestLoadConfigValid(t *testing.T) {
	config, err := LoadConfig(newLoader(validValues()))
	if err != nil {
		t.Fatalf("LoadConfig returned error: %v", err)
	}
	if config.MerchantEmail != "seed.merchant@example.com" {
		t.Fatalf("MerchantEmail = %q, want normalized email", config.MerchantEmail)
	}
	if config.ContactEmail != "seed.contact@example.com" {
		t.Fatalf("ContactEmail = %q, want normalized email", config.ContactEmail)
	}
	if config.BusinessRegNo != "PV 123456" {
		t.Fatalf("BusinessRegNo = %q, want normalized registration number", config.BusinessRegNo)
	}
	if config.PickupZoneCode != "CMB" {
		t.Fatalf("PickupZoneCode = %q, want CMB", config.PickupZoneCode)
	}
	if config.AppEnv != "development" || config.IdentityBaseURL != "http://localhost:8081" {
		t.Fatalf("unexpected config: %+v", config)
	}
}

func TestLoadConfigValidation(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(map[string]string)
	}{
		{name: "missing password", mutate: func(values map[string]string) { delete(values, "SEED_MERCHANT_PASSWORD") }},
		{name: "blank password", mutate: func(values map[string]string) { values["SEED_MERCHANT_PASSWORD"] = "   " }},
		{name: "weak password", mutate: func(values map[string]string) { values["SEED_MERCHANT_PASSWORD"] = "alllowercase" }},
		{name: "invalid merchant email", mutate: func(values map[string]string) { values["SEED_MERCHANT_EMAIL"] = "not-an-email" }},
		{name: "invalid contact email", mutate: func(values map[string]string) { values["SEED_MERCHANT_CONTACT_EMAIL"] = "not-an-email" }},
		{name: "invalid phone", mutate: func(values map[string]string) { values["SEED_MERCHANT_PHONE"] = "0771234567" }},
		{name: "short full name", mutate: func(values map[string]string) { values["SEED_MERCHANT_FULL_NAME"] = "A" }},
		{name: "invalid registration number", mutate: func(values map[string]string) { values["SEED_MERCHANT_BUSINESS_REG_NO"] = "ABCDEF" }},
		{name: "invalid identity URL", mutate: func(values map[string]string) { values["IDENTITY_BASE_URL"] = "identity:8081" }},
		{name: "missing internal token", mutate: func(values map[string]string) { delete(values, "INTERNAL_TOKEN") }},
		{name: "invalid app env", mutate: func(values map[string]string) { values["APP_ENV"] = "qa" }},
		{name: "blank zone", mutate: func(values map[string]string) { values["SEED_MERCHANT_PICKUP_ZONE_CODE"] = "  " }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			values := validValues()
			test.mutate(values)
			if _, err := LoadConfig(newLoader(values)); err == nil {
				t.Fatal("LoadConfig accepted an invalid configuration")
			}
		})
	}
}

func TestLoadConfigRefusesProduction(t *testing.T) {
	values := validValues()
	values["APP_ENV"] = "production"
	if _, err := LoadConfig(newLoader(values)); err == nil {
		t.Fatal("LoadConfig accepted APP_ENV=production")
	} else if !strings.Contains(err.Error(), "production") {
		t.Fatalf("error = %v, want a production refusal", err)
	}
}

func TestLoadConfigRefusesRemoteDatabase(t *testing.T) {
	values := validValues()
	values["DATABASE_URL"] = "postgres://swiftdrop:swiftdrop@db.example.com:5432/swiftdrop?sslmode=require"
	if _, err := LoadConfig(newLoader(values)); err == nil {
		t.Fatal("LoadConfig accepted a remote database by default")
	}

	values["ALLOW_REMOTE_SEED"] = "1"
	if _, err := LoadConfig(newLoader(values)); err != nil {
		t.Fatalf("LoadConfig rejected an explicitly allowed remote database: %v", err)
	}
}

func TestLoadConfigAllowsLoopbackDatabase(t *testing.T) {
	for _, host := range []string{"localhost", "127.0.0.1", "[::1]"} {
		values := validValues()
		values["DATABASE_URL"] = "postgres://swiftdrop:swiftdrop@" + host + ":5432/swiftdrop?sslmode=disable"
		if _, err := LoadConfig(newLoader(values)); err != nil {
			t.Fatalf("LoadConfig rejected loopback host %s: %v", host, err)
		}
	}
}

func TestLoadConfigDoesNotLeakSecrets(t *testing.T) {
	values := validValues()
	values["SEED_MERCHANT_PASSWORD"] = "Priv4teSeedPass"
	values["SEED_MERCHANT_BUSINESS_REG_NO"] = "ABCDEF"

	_, err := LoadConfig(newLoader(values))
	if err == nil {
		t.Fatal("LoadConfig accepted an invalid registration number")
	}
	message := err.Error()
	for _, secret := range []string{values["SEED_MERCHANT_PASSWORD"], values["INTERNAL_TOKEN"], values["DATABASE_URL"]} {
		if strings.Contains(message, secret) {
			t.Fatalf("error leaked a secret: %v", err)
		}
	}
}

func TestLoadConfigDoesNotLeakMalformedDatabaseURL(t *testing.T) {
	values := validValues()
	values["DATABASE_URL"] = "postgres://user:SecretPass@localhost:5432/%zz"

	_, err := LoadConfig(newLoader(values))
	if err == nil {
		t.Fatal("LoadConfig accepted a malformed DATABASE_URL")
	}
	if strings.Contains(err.Error(), "SecretPass") || strings.Contains(err.Error(), values["DATABASE_URL"]) {
		t.Fatalf("error leaked the database URL: %v", err)
	}
}
