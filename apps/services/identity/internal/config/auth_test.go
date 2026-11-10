package config

import (
	"net/http"
	"strings"
	"testing"
	"time"

	sharedconfig "github.com/Vineth-R/swiftdrop/packages/go-common/config"
)

const strongSecret = "a-long-random-signing-secret-for-tests-0123456789"

func authLoader(values map[string]string) sharedconfig.Loader {
	return sharedconfig.NewLoader(func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	})
}

func TestLoadAuthFrom_defaults(t *testing.T) {
	got, err := LoadAuthFrom(authLoader(map[string]string{"JWT_ACCESS_SECRET": strongSecret}), "development")
	if err != nil {
		t.Fatalf("LoadAuthFrom returned error: %v", err)
	}

	want := Auth{
		AccessSecret:              strongSecret,
		AccessTokenTTL:            15 * time.Minute,
		RefreshTokenTTL:           7 * 24 * time.Hour,
		OAuthStateTTL:             10 * time.Minute,
		PasswordResetTokenTTL:     30 * time.Minute,
		EmailVerificationTokenTTL: 30 * time.Minute,
		AccessCookieName:          "sd_access",
		RefreshCookieName:         "sd_refresh",
		CookieSecure:              false,
		CookieSameSite:            http.SameSiteLaxMode,
		MaxFailedLogins:           5,
		LockoutDuration:           15 * time.Minute,
	}
	if got != want {
		t.Fatalf("LoadAuthFrom() = %+v, want %+v", got, want)
	}
}

func TestLoadAuthFrom_overrides(t *testing.T) {
	got, err := LoadAuthFrom(authLoader(map[string]string{
		"JWT_ACCESS_SECRET":        strongSecret,
		"ACCESS_TOKEN_TTL":         "10m",
		"REFRESH_TOKEN_TTL":        "24h",
		"OAUTH_STATE_TTL":          "5m",
		"PASSWORD_RESET_TOKEN_TTL": "45m",
		"COOKIE_NAME":              "a_access",
		"COOKIE_REFRESH_NAME":      "a_refresh",
		"COOKIE_SECURE":            "true",
		"COOKIE_SAMESITE":          "Strict",
		"LOGIN_MAX_FAILURES":       "3",
		"LOGIN_LOCKOUT_DURATION":   "30m",
	}), "production")
	if err != nil {
		t.Fatalf("LoadAuthFrom returned error: %v", err)
	}
	if got.AccessTokenTTL != 10*time.Minute || got.RefreshTokenTTL != 24*time.Hour {
		t.Fatalf("lifetimes = %s / %s", got.AccessTokenTTL, got.RefreshTokenTTL)
	}
	if got.OAuthStateTTL != 5*time.Minute || got.PasswordResetTokenTTL != 45*time.Minute {
		t.Fatalf("oauth/reset lifetimes = %s / %s", got.OAuthStateTTL, got.PasswordResetTokenTTL)
	}
	if got.AccessCookieName != "a_access" || got.RefreshCookieName != "a_refresh" {
		t.Fatalf("cookie names = %q / %q", got.AccessCookieName, got.RefreshCookieName)
	}
	if !got.CookieSecure || got.CookieSameSite != http.SameSiteStrictMode {
		t.Fatalf("cookie flags = secure %t samesite %v", got.CookieSecure, got.CookieSameSite)
	}
	if got.MaxFailedLogins != 3 || got.LockoutDuration != 30*time.Minute {
		t.Fatalf("lockout = %d / %s", got.MaxFailedLogins, got.LockoutDuration)
	}
}

func TestLoadAuthFrom_errors(t *testing.T) {
	tests := []struct {
		name    string
		appEnv  string
		values  map[string]string
		wantErr string
	}{
		{
			name:    "missing secret",
			appEnv:  "development",
			values:  map[string]string{},
			wantErr: "JWT_ACCESS_SECRET",
		},
		{
			name:    "blank secret",
			appEnv:  "development",
			values:  map[string]string{"JWT_ACCESS_SECRET": "   "},
			wantErr: "JWT_ACCESS_SECRET",
		},
		{
			name:    "short secret outside development",
			appEnv:  "production",
			values:  map[string]string{"JWT_ACCESS_SECRET": "short"},
			wantErr: "at least 32 bytes",
		},
		{
			name:    "placeholder secret outside development",
			appEnv:  "staging",
			values:  map[string]string{"JWT_ACCESS_SECRET": "change-me-" + strings.Repeat("x", 40)},
			wantErr: "placeholder",
		},
		{
			name:    "refresh lifetime not longer than access lifetime",
			appEnv:  "development",
			values:  map[string]string{"JWT_ACCESS_SECRET": strongSecret, "ACCESS_TOKEN_TTL": "1h", "REFRESH_TOKEN_TTL": "1h"},
			wantErr: "REFRESH_TOKEN_TTL",
		},
		{
			name:    "bad lifetime",
			appEnv:  "development",
			values:  map[string]string{"JWT_ACCESS_SECRET": strongSecret, "ACCESS_TOKEN_TTL": "soon"},
			wantErr: "ACCESS_TOKEN_TTL",
		},
		{
			name:    "same cookie name twice",
			appEnv:  "development",
			values:  map[string]string{"JWT_ACCESS_SECRET": strongSecret, "COOKIE_NAME": "sd", "COOKIE_REFRESH_NAME": "sd"},
			wantErr: "must differ",
		},
		{
			name:    "bad secure flag",
			appEnv:  "development",
			values:  map[string]string{"JWT_ACCESS_SECRET": strongSecret, "COOKIE_SECURE": "maybe"},
			wantErr: "COOKIE_SECURE",
		},
		{
			name:    "production requires secure cookies",
			appEnv:  "production",
			values:  map[string]string{"JWT_ACCESS_SECRET": strongSecret},
			wantErr: "COOKIE_SECURE must be true in production",
		},
		{
			name:    "bad samesite",
			appEnv:  "development",
			values:  map[string]string{"JWT_ACCESS_SECRET": strongSecret, "COOKIE_SAMESITE": "sideways"},
			wantErr: "COOKIE_SAMESITE",
		},
		{
			name:    "samesite none without secure",
			appEnv:  "development",
			values:  map[string]string{"JWT_ACCESS_SECRET": strongSecret, "COOKIE_SAMESITE": "none"},
			wantErr: "requires COOKIE_SECURE",
		},
		{
			name:    "zero failures",
			appEnv:  "development",
			values:  map[string]string{"JWT_ACCESS_SECRET": strongSecret, "LOGIN_MAX_FAILURES": "0"},
			wantErr: "LOGIN_MAX_FAILURES",
		},
		{
			name:    "non numeric failures",
			appEnv:  "development",
			values:  map[string]string{"JWT_ACCESS_SECRET": strongSecret, "LOGIN_MAX_FAILURES": "five"},
			wantErr: "LOGIN_MAX_FAILURES",
		},
		{
			name:    "bad lockout duration",
			appEnv:  "development",
			values:  map[string]string{"JWT_ACCESS_SECRET": strongSecret, "LOGIN_LOCKOUT_DURATION": "-1m"},
			wantErr: "LOGIN_LOCKOUT_DURATION",
		},
		{
			name:    "oauth state lifetime too short",
			appEnv:  "development",
			values:  map[string]string{"JWT_ACCESS_SECRET": strongSecret, "OAUTH_STATE_TTL": "30s"},
			wantErr: "OAUTH_STATE_TTL",
		},
		{
			name:    "oauth state lifetime too long",
			appEnv:  "development",
			values:  map[string]string{"JWT_ACCESS_SECRET": strongSecret, "OAUTH_STATE_TTL": "2h"},
			wantErr: "OAUTH_STATE_TTL",
		},
		{
			name:    "reset token lifetime too short",
			appEnv:  "development",
			values:  map[string]string{"JWT_ACCESS_SECRET": strongSecret, "PASSWORD_RESET_TOKEN_TTL": "1m"},
			wantErr: "PASSWORD_RESET_TOKEN_TTL",
		},
		{
			name:    "reset token lifetime too long",
			appEnv:  "development",
			values:  map[string]string{"JWT_ACCESS_SECRET": strongSecret, "PASSWORD_RESET_TOKEN_TTL": "48h"},
			wantErr: "PASSWORD_RESET_TOKEN_TTL",
		},
		{
			name:    "access token lifetime zero",
			appEnv:  "development",
			values:  map[string]string{"JWT_ACCESS_SECRET": strongSecret, "ACCESS_TOKEN_TTL": "0s"},
			wantErr: "ACCESS_TOKEN_TTL",
		},
		{
			name:    "access token lifetime too long",
			appEnv:  "development",
			values:  map[string]string{"JWT_ACCESS_SECRET": strongSecret, "ACCESS_TOKEN_TTL": "48h"},
			wantErr: "ACCESS_TOKEN_TTL",
		},
		{
			name:    "refresh token lifetime too long",
			appEnv:  "development",
			values:  map[string]string{"JWT_ACCESS_SECRET": strongSecret, "REFRESH_TOKEN_TTL": "8761h"},
			wantErr: "REFRESH_TOKEN_TTL",
		},
		{
			name:    "lockout duration too long",
			appEnv:  "development",
			values:  map[string]string{"JWT_ACCESS_SECRET": strongSecret, "LOGIN_LOCKOUT_DURATION": "25h"},
			wantErr: "LOGIN_LOCKOUT_DURATION",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := LoadAuthFrom(authLoader(test.values), test.appEnv)
			if err == nil {
				t.Fatal("LoadAuthFrom returned no error")
			}
			if !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("error = %q, want it to contain %q", err.Error(), test.wantErr)
			}
			if strings.Contains(err.Error(), strongSecret) {
				t.Fatalf("error %q leaks the secret", err.Error())
			}
		})
	}
}

func TestLoadAuthFrom_developmentAllowsShortSecret(t *testing.T) {
	// Existing local .env files copied from the old example use a short
	// placeholder; development must keep starting with them.
	got, err := LoadAuthFrom(authLoader(map[string]string{"JWT_ACCESS_SECRET": "change-me-access-secret"}), "development")
	if err != nil {
		t.Fatalf("LoadAuthFrom returned error: %v", err)
	}
	if got.AccessSecret != "change-me-access-secret" {
		t.Fatalf("AccessSecret = %q", got.AccessSecret)
	}
}
