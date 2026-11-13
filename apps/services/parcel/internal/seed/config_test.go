package seed

import (
	"strings"
	"testing"

	sharedconfig "github.com/Vineth-R/swiftdrop/packages/go-common/config"
)

func validValues() map[string]string {
	return map[string]string{
		"APP_ENV":               "development",
		"DATABASE_URL":          "postgres://u:p@localhost:5432/db?sslmode=disable",
		"SEED_CUSTOMER_EMAIL":   "seed.customer@example.com",
		"SEED_COURIER_1_EMAIL":  "seed.courier.1@example.com",
		"SEED_COURIER_2_EMAIL":  "seed.courier.2@example.com",
		"SEED_DISPATCHER_EMAIL": "seed.dispatcher@example.com",
	}
}

func newLoader(values map[string]string) sharedconfig.Loader {
	return sharedconfig.NewLoader(func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	})
}

func TestLoadConfig(t *testing.T) {
	config, err := LoadConfig(newLoader(validValues()))
	if err != nil {
		t.Fatalf("LoadConfig returned error: %v", err)
	}
	if config.TrackingPrefix != "SDKDEMO" {
		t.Fatalf("tracking prefix = %q", config.TrackingPrefix)
	}
}

func TestValidateTarget(t *testing.T) {
	tests := []struct {
		name        string
		appEnv      string
		url         string
		allowRemote bool
		wantErr     bool
	}{
		{name: "local", appEnv: "development", url: "postgres://u:p@localhost:5432/db"},
		{name: "compose development", appEnv: "development", url: "postgres://u:p@postgres:5432/db"},
		{name: "production refused", appEnv: "production", url: "postgres://u:p@localhost:5432/db", wantErr: true},
		{name: "remote refused", appEnv: "development", url: "postgres://u:p@db.example.com:5432/db", wantErr: true},
		{name: "remote explicitly allowed", appEnv: "development", url: "postgres://u:p@db.example.com:5432/db", allowRemote: true},
		{name: "compose refused outside development", appEnv: "staging", url: "postgres://u:p@postgres:5432/db", wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateTarget(test.appEnv, test.url, test.allowRemote)
			if (err != nil) != test.wantErr {
				t.Fatalf("ValidateTarget error = %v, wantErr %v", err, test.wantErr)
			}
			if err != nil && strings.Contains(err.Error(), "u:p") {
				t.Fatalf("error leaked credentials: %v", err)
			}
		})
	}
}

func TestDemoParcelsCoverEveryStatus(t *testing.T) {
	want := []string{"CREATED", "PICKED_UP", "IN_TRANSIT", "OUT_FOR_DELIVERY", "DELIVERED", "FAILED"}
	seen := map[string]bool{}
	for _, parcel := range demoParcels {
		seen[parcel.status] = true
	}
	for _, status := range want {
		if !seen[status] {
			t.Errorf("demo parcels do not cover %s", status)
		}
	}
}
