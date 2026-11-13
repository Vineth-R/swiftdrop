// Package seed implements the local deterministic parcel demo seed command.
// It runs only when an operator invokes it; it is not part of service startup.
package seed

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	sharedconfig "github.com/Vineth-R/swiftdrop/packages/go-common/config"
	"github.com/Vineth-R/swiftdrop/packages/go-common/validation"
)

const (
	defaultTrackingPrefix = "SDKDEMO"
	maxTrackingPrefixLen  = 12
)

// Config is the validated parcel demo-seed configuration.
type Config struct {
	AppEnv      string
	DatabaseURL string

	CustomerEmail     string
	CourierOneEmail   string
	CourierTwoEmail   string
	DispatcherEmail   string
	TrackingPrefix    string
	AllowRemoteTarget bool
}

// LoadConfig reads and validates the parcel demo seed from the environment.
// It never returns a configuration that targets production or, by default, a
// remote database.
func LoadConfig(loader sharedconfig.Loader) (Config, error) {
	appEnv, err := loader.EnumFold("APP_ENV", "development", "development", "staging", "production")
	if err != nil {
		return Config{}, err
	}

	databaseURL, err := loader.Required("DATABASE_URL")
	if err != nil {
		return Config{}, err
	}

	customerEmail, err := requiredEmail(loader, "SEED_CUSTOMER_EMAIL")
	if err != nil {
		return Config{}, err
	}
	courierOneEmail, err := requiredEmail(loader, "SEED_COURIER_1_EMAIL")
	if err != nil {
		return Config{}, err
	}
	courierTwoEmail, err := requiredEmail(loader, "SEED_COURIER_2_EMAIL")
	if err != nil {
		return Config{}, err
	}
	dispatcherEmail, err := requiredEmail(loader, "SEED_DISPATCHER_EMAIL")
	if err != nil {
		return Config{}, err
	}

	prefix := strings.ToUpper(strings.TrimSpace(loader.String("SEED_PARCEL_TRACKING_PREFIX", defaultTrackingPrefix)))
	if prefix == "" || len(prefix) > maxTrackingPrefixLen || !validPrefix(prefix) {
		return Config{}, fmt.Errorf("SEED_PARCEL_TRACKING_PREFIX must be 1 to %d letters or digits", maxTrackingPrefixLen)
	}

	allowRemote := strings.EqualFold(loader.String("ALLOW_REMOTE_SEED", ""), "1") ||
		strings.EqualFold(loader.String("ALLOW_REMOTE_SEED", ""), "true")

	config := Config{
		AppEnv:            appEnv,
		DatabaseURL:       databaseURL,
		CustomerEmail:     customerEmail,
		CourierOneEmail:   courierOneEmail,
		CourierTwoEmail:   courierTwoEmail,
		DispatcherEmail:   dispatcherEmail,
		TrackingPrefix:    prefix,
		AllowRemoteTarget: allowRemote,
	}
	if err := config.Validate(); err != nil {
		return Config{}, err
	}
	return config, nil
}

// Validate checks the complete configuration before any database work.
func (c Config) Validate() error {
	if strings.EqualFold(strings.TrimSpace(c.AppEnv), "production") {
		return errors.New("refusing to run the parcel demo seed when APP_ENV=production")
	}
	if err := ValidateTarget(c.AppEnv, c.DatabaseURL, c.AllowRemoteTarget); err != nil {
		return err
	}
	for name, email := range map[string]string{
		"SEED_CUSTOMER_EMAIL":   c.CustomerEmail,
		"SEED_COURIER_1_EMAIL":  c.CourierOneEmail,
		"SEED_COURIER_2_EMAIL":  c.CourierTwoEmail,
		"SEED_DISPATCHER_EMAIL": c.DispatcherEmail,
	} {
		if !validation.Email(strings.TrimSpace(email)) {
			return fmt.Errorf("%s must be a valid email address", name)
		}
	}
	if c.CustomerEmail == c.CourierOneEmail || c.CustomerEmail == c.CourierTwoEmail ||
		c.CourierOneEmail == c.CourierTwoEmail {
		return errors.New("demo seed emails must be distinct")
	}
	if len(c.TrackingPrefix) == 0 || len(c.TrackingPrefix) > maxTrackingPrefixLen || !validPrefix(c.TrackingPrefix) {
		return fmt.Errorf("tracking prefix must be 1 to %d letters or digits", maxTrackingPrefixLen)
	}
	return nil
}

// ValidateTarget refuses production and, by default, a non-local database.
// Errors never include the database URL or a credential.
func ValidateTarget(appEnv, databaseURL string, allowRemote bool) error {
	if strings.EqualFold(strings.TrimSpace(appEnv), "production") {
		return errors.New("refusing to run the parcel demo seed when APP_ENV=production")
	}
	parsed, err := url.Parse(strings.TrimSpace(databaseURL))
	if err != nil || parsed.Hostname() == "" {
		return errors.New("DATABASE_URL must be a valid PostgreSQL URL with a host")
	}
	host := strings.ToLower(parsed.Hostname())
	if isLocalHost(host) || (host == "postgres" && appEnv == "development") || allowRemote {
		return nil
	}
	return fmt.Errorf("refusing to seed a non-local database host %q; set ALLOW_REMOTE_SEED=1 only for a disposable database", host)
}

func isLocalHost(host string) bool {
	switch host {
	case "localhost", "127.0.0.1", "::1":
		return true
	}
	return strings.HasSuffix(host, ".local")
}

func requiredEmail(loader sharedconfig.Loader, key string) (string, error) {
	value, err := loader.Required(key)
	if err != nil {
		return "", err
	}
	value = strings.TrimSpace(value)
	if !validation.Email(value) {
		return "", fmt.Errorf("%s must be a valid email address", key)
	}
	return value, nil
}

func validPrefix(prefix string) bool {
	for _, char := range prefix {
		if (char < 'A' || char > 'Z') && (char < '0' || char > '9') {
			return false
		}
	}
	return true
}
