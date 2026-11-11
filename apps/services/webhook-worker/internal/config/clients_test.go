package config

import (
	"strings"
	"testing"
	"time"

	sharedconfig "github.com/Vineth-R/swiftdrop/packages/go-common/config"
)

func loaderOf(values map[string]string) sharedconfig.Loader {
	return sharedconfig.NewLoader(func(key string) (string, bool) {
		v, ok := values[key]
		return v, ok
	})
}

func TestLoadClientsDefaults(t *testing.T) {
	c, err := LoadClients(loaderOf(map[string]string{"INTERNAL_TOKEN": "a-long-enough-dev-token"}), "development")
	if err != nil {
		t.Fatalf("LoadClients() error: %v", err)
	}
	if c.MerchantBaseURL != "http://merchant:8084" || c.ParcelBaseURL != "http://parcel:8082" ||
		c.ClientTimeout != 5*time.Second || c.InternalToken == "" || c.InternalTokenProblem != "" {
		t.Fatalf("clients = %+v", c)
	}
}

func TestLoadClientsDevelopmentMissingTokenKeepsRunning(t *testing.T) {
	c, err := LoadClients(loaderOf(nil), "development")
	if err != nil {
		t.Fatalf("LoadClients() error: %v", err)
	}
	if c.InternalToken != "" || c.InternalTokenProblem == "" {
		t.Fatalf("clients = %+v", c)
	}
}

func TestLoadClientsProductionNeedsStrongToken(t *testing.T) {
	if _, err := LoadClients(loaderOf(map[string]string{"INTERNAL_TOKEN": "short"}), "production"); err == nil {
		t.Fatal("a weak token must stop the service")
	}
	if _, err := LoadClients(loaderOf(nil), "production"); err == nil {
		t.Fatal("a missing token must stop the service")
	}
	token := strings.Repeat("k", 40)
	if _, err := LoadClients(loaderOf(map[string]string{"INTERNAL_TOKEN": token}), "production"); err != nil {
		t.Fatalf("LoadClients() error: %v", err)
	}
}

func TestLoadClientsRejectsBadOrigins(t *testing.T) {
	for _, bad := range []string{"merchant:8084", "ftp://m", "http://u:p@m", "http://m/path", "http://m?x=1"} {
		_, err := LoadClients(loaderOf(map[string]string{
			"INTERNAL_TOKEN": "a-long-enough-dev-token", "MERCHANT_BASE_URL": bad,
		}), "development")
		if err == nil {
			t.Errorf("origin %q accepted", bad)
		}
	}
	if _, err := LoadClients(loaderOf(map[string]string{
		"INTERNAL_TOKEN": "a-long-enough-dev-token", "INTERNAL_CLIENT_TIMEOUT": "0s",
	}), "development"); err == nil {
		t.Error("zero timeout accepted")
	}
}
