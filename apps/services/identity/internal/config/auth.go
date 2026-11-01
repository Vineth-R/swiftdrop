package config

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
	sharedconfig "github.com/Vineth-R/swiftdrop/packages/go-common/config"
)

// Auth is the session and login-protection configuration of the identity
// service (API v1.1 sections 1.2 and 2.1 to 2.4). It is loaded separately from
// Config so the authentication settings stay in one place.
type Auth struct {
	AccessSecret      string
	AccessTokenTTL    time.Duration
	RefreshTokenTTL   time.Duration
	AccessCookieName  string
	RefreshCookieName string
	CookieSecure      bool
	CookieSameSite    http.SameSite
	MaxFailedLogins   int
	LockoutDuration   time.Duration
}

// LoadAuth reads the authentication settings from the process environment.
// appEnv is the already validated APP_ENV value.
func LoadAuth(appEnv string) (Auth, error) {
	return LoadAuthFrom(sharedconfig.Load(), appEnv)
}

// LoadAuthFrom reads the authentication settings from loader. It fails fast on
// a missing signing secret; outside development it also rejects a short or
// placeholder secret.
func LoadAuthFrom(loader sharedconfig.Loader, appEnv string) (Auth, error) {
	secret, err := loader.Required("JWT_ACCESS_SECRET")
	if err != nil {
		return Auth{}, err
	}
	if appEnv != "development" {
		if err := auth.CheckSecretStrength(secret); err != nil {
			return Auth{}, fmt.Errorf("JWT_ACCESS_SECRET: %w", err)
		}
		if strings.HasPrefix(secret, "change-me") {
			return Auth{}, errors.New("JWT_ACCESS_SECRET must not be the example placeholder")
		}
	}

	accessTTL, err := loader.Duration("ACCESS_TOKEN_TTL", 15*time.Minute)
	if err != nil {
		return Auth{}, err
	}
	refreshTTL, err := loader.Duration("REFRESH_TOKEN_TTL", 7*24*time.Hour)
	if err != nil {
		return Auth{}, err
	}
	if refreshTTL <= accessTTL {
		return Auth{}, errors.New("REFRESH_TOKEN_TTL must be longer than ACCESS_TOKEN_TTL")
	}

	accessCookie := loader.String("COOKIE_NAME", "sd_access")
	refreshCookie := loader.String("COOKIE_REFRESH_NAME", "sd_refresh")
	if accessCookie == refreshCookie {
		return Auth{}, errors.New("COOKIE_NAME and COOKIE_REFRESH_NAME must differ")
	}

	secure, err := strconv.ParseBool(loader.String("COOKIE_SECURE", "false"))
	if err != nil {
		return Auth{}, fmt.Errorf("COOKIE_SECURE must be true or false: %w", err)
	}
	sameSite, err := parseSameSite(loader)
	if err != nil {
		return Auth{}, err
	}
	if sameSite == http.SameSiteNoneMode && !secure {
		return Auth{}, errors.New("COOKIE_SAMESITE=none requires COOKIE_SECURE=true")
	}

	maxFailures, err := loader.Int("LOGIN_MAX_FAILURES", 5)
	if err != nil {
		return Auth{}, err
	}
	if maxFailures < 1 || maxFailures > 100 {
		return Auth{}, errors.New("LOGIN_MAX_FAILURES must be between 1 and 100")
	}
	lockoutDuration, err := loader.Duration("LOGIN_LOCKOUT_DURATION", 15*time.Minute)
	if err != nil {
		return Auth{}, err
	}

	return Auth{
		AccessSecret:      secret,
		AccessTokenTTL:    accessTTL,
		RefreshTokenTTL:   refreshTTL,
		AccessCookieName:  accessCookie,
		RefreshCookieName: refreshCookie,
		CookieSecure:      secure,
		CookieSameSite:    sameSite,
		MaxFailedLogins:   maxFailures,
		LockoutDuration:   lockoutDuration,
	}, nil
}

func parseSameSite(loader sharedconfig.Loader) (http.SameSite, error) {
	value, err := loader.EnumFold("COOKIE_SAMESITE", "lax", "lax", "strict", "none")
	if err != nil {
		return 0, err
	}
	switch value {
	case "strict":
		return http.SameSiteStrictMode, nil
	case "none":
		return http.SameSiteNoneMode, nil
	default:
		return http.SameSiteLaxMode, nil
	}
}
