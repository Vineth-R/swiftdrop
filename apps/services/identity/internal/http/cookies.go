package servicehttp

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Vineth-R/swiftdrop/apps/services/identity/internal/service"
)

// refreshCookiePath limits the refresh cookie to the auth endpoints, so it is
// not sent with every API call. It is the public path behind the gateway.
const refreshCookiePath = "/api/v1/auth"

// oauthBindingCookiePath limits the browser-binding cookie to the Google
// endpoints. It is the public path behind the gateway.
const oauthBindingCookiePath = "/api/v1/auth/google"

// oauthBindingCookieMaxAge matches the 10 minute OAuth state lifetime.
const oauthBindingCookieMaxAge = 600

// CookieConfig describes the session cookies (API v1.1 section 1.2): an
// HttpOnly access cookie and an HttpOnly refresh cookie.
type CookieConfig struct {
	AccessName  string
	RefreshName string
	Secure      bool
	SameSite    http.SameSite
}

// SetSession writes the access and refresh cookies for a session. Other login
// flows in this service, such as Google sign-in, use it so every login sets
// identical cookies.
func (cfg CookieConfig) SetSession(c *gin.Context, session service.Session) {
	cfg.set(c, cfg.AccessName, session.AccessToken, "/", seconds(time.Until(session.AccessExpiresAt)))
	cfg.set(c, cfg.RefreshName, session.RefreshToken, refreshCookiePath, seconds(time.Until(session.RefreshExpiresAt)))
}

// Clear expires both cookies. The attributes must match those used to set
// them, otherwise the browser keeps the old cookie.
func (cfg CookieConfig) Clear(c *gin.Context) {
	cfg.set(c, cfg.AccessName, "", "/", -1)
	cfg.set(c, cfg.RefreshName, "", refreshCookiePath, -1)
}

// RefreshToken returns the refresh token sent as a cookie, if any.
func (cfg CookieConfig) RefreshToken(c *gin.Context) string {
	value, err := c.Cookie(cfg.RefreshName)
	if err != nil {
		return ""
	}
	return value
}

// SetOAuthBinding writes the per-attempt browser-binding cookie. It always
// uses SameSite=Lax because the Google callback is a cross-site top-level
// navigation; a stricter session-cookie policy must not break the callback.
func (cfg CookieConfig) SetOAuthBinding(c *gin.Context, name, value string) {
	cfg.setOAuthBinding(c, name, value, oauthBindingCookieMaxAge)
}

// ClearOAuthBinding expires a browser-binding cookie after callback handling.
func (cfg CookieConfig) ClearOAuthBinding(c *gin.Context, name string) {
	cfg.setOAuthBinding(c, name, "", -1)
}

func (cfg CookieConfig) setOAuthBinding(c *gin.Context, name, value string, maxAge int) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     oauthBindingCookiePath,
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   cfg.Secure,
		SameSite: http.SameSiteLaxMode,
	})
}

func (cfg CookieConfig) set(c *gin.Context, name, value, path string, maxAge int) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     path,
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   cfg.Secure,
		SameSite: cfg.SameSite,
	})
}

// seconds converts a lifetime to a cookie Max-Age. A zero or negative lifetime
// becomes -1, which deletes the cookie instead of creating a session cookie.
func seconds(lifetime time.Duration) int {
	if lifetime <= 0 {
		return -1
	}
	return int(lifetime / time.Second)
}
