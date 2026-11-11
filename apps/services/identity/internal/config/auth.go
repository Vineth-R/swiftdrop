package config

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	sharedconfig "github.com/Vineth-R/swiftdrop/packages/go-common/config"
)

// Auth is the session and login-protection configuration of the identity
// service (API v1.1 sections 1.2 and 2.1 to 2.4). It is loaded separately from
// Config so the authentication settings stay in one place.
type Auth struct {
	AccessSecret              string
	AccessTokenTTL            time.Duration
	RefreshTokenTTL           time.Duration
	OAuthStateTTL             time.Duration
	PasswordResetTokenTTL     time.Duration
	EmailVerificationTokenTTL time.Duration
	AccessCookieName          string
	RefreshCookieName         string
	CookieSecure              bool
	CookieSameSite            http.SameSite
	MaxFailedLogins           int
	LockoutDuration           time.Duration
}

const (
	defaultAccessTokenTTL            = 15 * time.Minute
	defaultRefreshTokenTTL           = 7 * 24 * time.Hour
	defaultOAuthStateTTL             = 10 * time.Minute
	defaultPasswordResetTokenTTL     = 30 * time.Minute
	defaultEmailVerificationTokenTTL = 30 * time.Minute

	maxAccessTokenTTL            = 24 * time.Hour
	maxRefreshTokenTTL           = 365 * 24 * time.Hour
	minOAuthStateTTL             = time.Minute
	maxOAuthStateTTL             = time.Hour
	minPasswordResetTokenTTL     = 5 * time.Minute
	maxPasswordResetTokenTTL     = 24 * time.Hour
	minEmailVerificationTokenTTL = 5 * time.Minute
	maxEmailVerificationTokenTTL = 24 * time.Hour
	maxLockoutDuration           = 24 * time.Hour
)

// LoadAuth reads the authentication settings from the process environment.
// appEnv is the already validated APP_ENV value.
func LoadAuth(appEnv string) (Auth, error) {
	return LoadAuthFrom(sharedconfig.Load(), appEnv)
}

// LoadAuthFrom reads the authentication settings from loader. It fails fast on
// a missing signing secret; outside development it also rejects a short or
// placeholder secret.
func LoadAuthFrom(loader sharedconfig.Loader, appEnv string) (Auth, error) {
	secret, err := sharedconfig.LoadAccessSecret(loader, appEnv, true)
	if err != nil {
		return Auth{}, err
	}

	accessTTL, err := loader.Duration("ACCESS_TOKEN_TTL", defaultAccessTokenTTL)
	if err != nil {
		return Auth{}, err
	}
	if err := validateTTL("ACCESS_TOKEN_TTL", accessTTL, time.Second, maxAccessTokenTTL); err != nil {
		return Auth{}, err
	}
	refreshTTL, err := loader.Duration("REFRESH_TOKEN_TTL", defaultRefreshTokenTTL)
	if err != nil {
		return Auth{}, err
	}
	if err := validateTTL("REFRESH_TOKEN_TTL", refreshTTL, time.Second, maxRefreshTokenTTL); err != nil {
		return Auth{}, err
	}
	if refreshTTL <= accessTTL {
		return Auth{}, errors.New("REFRESH_TOKEN_TTL must be longer than ACCESS_TOKEN_TTL")
	}
	oauthStateTTL, err := loader.Duration("OAUTH_STATE_TTL", defaultOAuthStateTTL)
	if err != nil {
		return Auth{}, err
	}
	if err := validateTTL("OAUTH_STATE_TTL", oauthStateTTL, minOAuthStateTTL, maxOAuthStateTTL); err != nil {
		return Auth{}, err
	}
	passwordResetTTL, err := loader.Duration("PASSWORD_RESET_TOKEN_TTL", defaultPasswordResetTokenTTL)
	if err != nil {
		return Auth{}, err
	}
	if err := validateTTL("PASSWORD_RESET_TOKEN_TTL", passwordResetTTL, minPasswordResetTokenTTL, maxPasswordResetTokenTTL); err != nil {
		return Auth{}, err
	}
	emailVerificationTTL, err := loader.Duration("EMAIL_VERIFICATION_TOKEN_TTL", defaultEmailVerificationTokenTTL)
	if err != nil {
		return Auth{}, err
	}
	if err := validateTTL("EMAIL_VERIFICATION_TOKEN_TTL", emailVerificationTTL, minEmailVerificationTokenTTL, maxEmailVerificationTokenTTL); err != nil {
		return Auth{}, err
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
	// Session cookies carry tokens. Production must not start with them
	// traversing plain HTTP. Local development intentionally uses HTTP, so the
	// insecure default remains available there.
	if appEnv == "production" && !secure {
		return Auth{}, errors.New("COOKIE_SECURE must be true in production")
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
	if err := validateTTL("LOGIN_LOCKOUT_DURATION", lockoutDuration, time.Second, maxLockoutDuration); err != nil {
		return Auth{}, err
	}

	return Auth{
		AccessSecret:              secret,
		AccessTokenTTL:            accessTTL,
		RefreshTokenTTL:           refreshTTL,
		OAuthStateTTL:             oauthStateTTL,
		PasswordResetTokenTTL:     passwordResetTTL,
		EmailVerificationTokenTTL: emailVerificationTTL,
		AccessCookieName:          accessCookie,
		RefreshCookieName:         refreshCookie,
		CookieSecure:              secure,
		CookieSameSite:            sameSite,
		MaxFailedLogins:           maxFailures,
		LockoutDuration:           lockoutDuration,
	}, nil
}

func validateTTL(name string, value, min, max time.Duration) error {
	if value < min || value > max {
		return fmt.Errorf("%s must be between %s and %s", name, min, max)
	}
	return nil
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
