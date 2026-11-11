package config

import (
	"errors"
	"net/url"
	"strings"
	"time"

	sharedconfig "github.com/Vineth-R/swiftdrop/packages/go-common/config"
)

// Clients holds the settings the worker needs to call the Merchant and Parcel
// services (internal APIs, API v1.1 section 13). It is loaded separately from
// Config so the queue settings stay untouched.
type Clients struct {
	MerchantBaseURL string
	ParcelBaseURL   string
	// InternalToken is empty when it is missing or unusable; the reason is in
	// InternalTokenProblem (never the token itself).
	InternalToken        string
	InternalTokenProblem string
	// ClientTimeout bounds one internal API call (not the merchant's endpoint).
	ClientTimeout time.Duration
}

// LoadClients reads the internal client settings. Outside development a set
// but weak INTERNAL_TOKEN stops the service, like the other services.
func LoadClients(loader sharedconfig.Loader, appEnv string) (Clients, error) {
	token, problem := sharedconfig.LoadInternalToken(loader, appEnv)
	if problem != "" && appEnv != "development" {
		return Clients{}, errors.New(problem)
	}

	merchantURL, err := serviceOrigin(loader.String("MERCHANT_BASE_URL", "http://merchant:8084"), "MERCHANT_BASE_URL")
	if err != nil {
		return Clients{}, err
	}
	parcelURL, err := serviceOrigin(loader.String("PARCEL_BASE_URL", "http://parcel:8082"), "PARCEL_BASE_URL")
	if err != nil {
		return Clients{}, err
	}

	timeout, err := loader.Duration("INTERNAL_CLIENT_TIMEOUT", 5*time.Second)
	if err != nil {
		return Clients{}, err
	}
	if timeout < time.Millisecond {
		return Clients{}, errors.New("INTERNAL_CLIENT_TIMEOUT must be at least 1ms")
	}

	return Clients{
		MerchantBaseURL:      merchantURL,
		ParcelBaseURL:        parcelURL,
		InternalToken:        token,
		InternalTokenProblem: problem,
		ClientTimeout:        timeout,
	}, nil
}

// serviceOrigin accepts only a bare http(s) origin, the same rule the parcel
// service applies to its own service URLs.
func serviceOrigin(raw, name string) (string, error) {
	trimmed := strings.TrimRight(raw, "/")
	u, err := url.Parse(trimmed)
	if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") ||
		u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" {
		return "", errors.New(name + " must be an HTTP(S) service origin")
	}
	return trimmed, nil
}
