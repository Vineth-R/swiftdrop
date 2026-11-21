package seed

import (
	"strings"
	"testing"

	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
	sharedconfig "github.com/Vineth-R/swiftdrop/packages/go-common/config"
)

func validValues() map[string]string {
	return map[string]string{
		"SEED_ADMIN_EMAIL":         "seed.admin@example.com",
		"SEED_ADMIN_PASSWORD":      "AdminPass1!",
		"SEED_DISPATCHER_EMAIL":    "seed.dispatcher@example.com",
		"SEED_DISPATCHER_PASSWORD": "DispatcherPass1!",
		"SEED_COURIER_1_EMAIL":     "seed.courier.1@example.com",
		"SEED_COURIER_1_PASSWORD":  "CourierOnePass1!",
		"SEED_COURIER_2_EMAIL":     "seed.courier.2@example.com",
		"SEED_COURIER_2_PASSWORD":  "CourierTwoPass1!",
		"SEED_CUSTOMER_EMAIL":      "seed.customer@example.com",
		"SEED_CUSTOMER_PASSWORD":   "CustomerPass1!",
	}
}

func newLoader(values map[string]string) sharedconfig.Loader {
	return sharedconfig.NewLoader(func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	})
}

func TestLoadConfig(t *testing.T) {
	values := validValues()
	values["SEED_ADMIN_EMAIL"] = "  SEED.ADMIN@EXAMPLE.COM  "

	config, err := LoadConfig(newLoader(values))
	if err != nil {
		t.Fatalf("LoadConfig returned error: %v", err)
	}
	if len(config.Accounts) != 5 {
		t.Fatalf("accounts = %d, want 5", len(config.Accounts))
	}
	if config.Accounts[0].Role != auth.RoleAdmin || config.Accounts[0].Email != "seed.admin@example.com" {
		t.Fatalf("admin = %+v", config.Accounts[0])
	}
	if config.Accounts[1].Role != auth.RoleDispatcher {
		t.Fatalf("dispatcher = %+v", config.Accounts[1])
	}
	if config.Accounts[2].Role != auth.RoleCourier || config.Accounts[2].HomeZoneCode != "CMB" || config.Accounts[2].VehicleType != "BIKE" {
		t.Fatalf("courier 1 = %+v", config.Accounts[2])
	}
	if config.Accounts[3].Role != auth.RoleCourier || config.Accounts[3].HomeZoneCode != "KDY" || config.Accounts[3].VehicleType != "VAN" {
		t.Fatalf("courier 2 = %+v", config.Accounts[3])
	}
	if config.Accounts[4].Role != auth.RoleCustomer || config.Accounts[4].FullName != "Demo Customer" {
		t.Fatalf("customer = %+v", config.Accounts[4])
	}
}

func TestLoadConfigMissingVariables(t *testing.T) {
	for _, key := range []string{
		"SEED_ADMIN_EMAIL", "SEED_ADMIN_PASSWORD",
		"SEED_DISPATCHER_EMAIL", "SEED_DISPATCHER_PASSWORD",
		"SEED_COURIER_1_EMAIL", "SEED_COURIER_1_PASSWORD",
		"SEED_COURIER_2_EMAIL", "SEED_COURIER_2_PASSWORD",
		"SEED_CUSTOMER_EMAIL", "SEED_CUSTOMER_PASSWORD",
	} {
		t.Run(key, func(t *testing.T) {
			values := validValues()
			delete(values, key)
			_, err := LoadConfig(newLoader(values))
			if err == nil {
				t.Fatalf("LoadConfig accepted missing %s", key)
			}
			if !strings.Contains(err.Error(), key) {
				t.Fatalf("error does not name %s: %v", key, err)
			}
		})
	}
}

func TestLoadConfigRejectsInvalidEmailWithoutLeakingValue(t *testing.T) {
	values := validValues()
	values["SEED_ADMIN_EMAIL"] = "not-an-email"

	_, err := LoadConfig(newLoader(values))
	if err == nil {
		t.Fatal("LoadConfig accepted an invalid email")
	}
	if !strings.Contains(err.Error(), "SEED_ADMIN_EMAIL") {
		t.Fatalf("error does not name the field: %v", err)
	}
	if strings.Contains(err.Error(), "not-an-email") {
		t.Fatalf("error leaked the email value: %v", err)
	}
}

func TestLoadConfigRejectsWeakPasswordWithoutLeakingValue(t *testing.T) {
	values := validValues()
	values["SEED_ADMIN_PASSWORD"] = "NoDigitsHere"

	_, err := LoadConfig(newLoader(values))
	if err == nil {
		t.Fatal("LoadConfig accepted a weak password")
	}
	if !strings.Contains(err.Error(), "SEED_ADMIN_PASSWORD") {
		t.Fatalf("error does not name the field: %v", err)
	}
	if strings.Contains(err.Error(), "NoDigitsHere") {
		t.Fatalf("error leaked the password value: %v", err)
	}
}

func TestLoadConfigRejectsDuplicateEmailsWithoutLeakingValue(t *testing.T) {
	values := validValues()
	values["SEED_COURIER_1_EMAIL"] = "  SEED.ADMIN@EXAMPLE.COM "

	_, err := LoadConfig(newLoader(values))
	if err == nil {
		t.Fatal("LoadConfig accepted duplicate emails")
	}
	if !strings.Contains(err.Error(), "unique") {
		t.Fatalf("error does not explain uniqueness: %v", err)
	}
	if strings.Contains(strings.ToLower(err.Error()), "seed.admin@example.com") {
		t.Fatalf("error leaked the email value: %v", err)
	}
}

func TestConfigValidateRejectsBlankCourierZone(t *testing.T) {
	config, err := LoadConfig(newLoader(validValues()))
	if err != nil {
		t.Fatal(err)
	}
	config.Accounts[2].HomeZoneCode = "   "
	if err := config.Validate(); err == nil {
		t.Fatal("Validate accepted a blank courier zone")
	} else if !strings.Contains(err.Error(), "home zone") {
		t.Fatalf("error = %v", err)
	}
}

func TestConfigValidateRejectsWrongRoleCounts(t *testing.T) {
	config, err := LoadConfig(newLoader(validValues()))
	if err != nil {
		t.Fatal(err)
	}
	config.Accounts[1].Role = auth.RoleAdmin
	if err := config.Validate(); err == nil {
		t.Fatal("Validate accepted the wrong role counts")
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
		{name: "local development", appEnv: "development", url: "postgres://u:p@localhost:5432/db?sslmode=disable"},
		{name: "compose service in development", appEnv: "development", url: "postgres://u:p@postgres:5432/db?sslmode=disable"},
		{name: "loopback v6", appEnv: "staging", url: "postgres://u:p@[::1]:5432/db"},
		{name: "production refused", appEnv: "production", url: "postgres://u:p@localhost:5432/db", wantErr: true},
		{name: "remote refused", appEnv: "development", url: "postgres://u:p@db.example.com:5432/db", wantErr: true},
		{name: "remote allowed explicitly", appEnv: "development", url: "postgres://u:p@db.example.com:5432/db", allowRemote: true},
		{name: "compose service refused outside development", appEnv: "staging", url: "postgres://u:p@postgres:5432/db", wantErr: true},
		{name: "malformed url", appEnv: "development", url: "not-a-url", wantErr: true},
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
