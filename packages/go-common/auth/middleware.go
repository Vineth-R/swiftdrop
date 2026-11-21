package auth

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/Vineth-R/swiftdrop/packages/go-common/httpx"
)

const claimsContextKey = "auth.claims"

// Verifier validates an access token. *TokenManager satisfies it.
type Verifier interface {
	Verify(token string) (Claims, error)
}

// Authenticate verifies the caller's access token and stores its claims on the
// request. The token is read from an "Authorization: Bearer" header when one is
// sent, otherwise from the named cookie (API v1.1 section 1.2). A request with
// no usable token is answered 401 and never reaches the next handler.
func Authenticate(verifier Verifier, cookieName string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if AuthenticateRequest(c, verifier, cookieName) {
			c.Next()
		}
	}
}

// AuthenticateRequest verifies the caller's token and stores the claims without
// advancing the Gin chain. It lets a route choose between session and API-key
// authentication without nesting c.Next() calls. The caller decides whether to
// continue; a false result means a JSON error has already been written.
func AuthenticateRequest(c *gin.Context, verifier Verifier, cookieName string) bool {
	token, ok := extractToken(c, cookieName)
	if !ok {
		httpx.Error(c, http.StatusUnauthorized, "UNAUTHENTICATED", "authentication required")
		return false
	}

	claims, err := verifier.Verify(token)
	if err != nil {
		if errors.Is(err, ErrTokenExpired) {
			httpx.Error(c, http.StatusUnauthorized, "TOKEN_EXPIRED", "access token expired")
			return false
		}
		httpx.Error(c, http.StatusUnauthorized, "UNAUTHENTICATED", "authentication required")
		return false
	}

	c.Set(claimsContextKey, claims)
	return true
}

// RequireRoles allows only callers whose role satisfies at least one of roles
// (ADMIN satisfies DISPATCHER). It must run after Authenticate. With no roles
// it denies every caller.
func RequireRoles(roles ...Role) gin.HandlerFunc {
	return func(c *gin.Context) {
		claims, ok := ClaimsFrom(c)
		if !ok {
			// Authenticate did not run: a routing mistake, not a client error.
			httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
			return
		}

		for _, required := range roles {
			if claims.Role.Satisfies(required) {
				c.Next()
				return
			}
		}
		httpx.Error(c, http.StatusForbidden, "FORBIDDEN", "not allowed")
	}
}

// ClaimsFrom returns the claims stored by Authenticate. The boolean is false
// when the request is unauthenticated, so callers must fail closed.
func ClaimsFrom(c *gin.Context) (Claims, bool) {
	value, exists := c.Get(claimsContextKey)
	if !exists {
		return Claims{}, false
	}
	claims, ok := value.(Claims)
	return claims, ok
}

func extractToken(c *gin.Context, cookieName string) (string, bool) {
	if header := c.GetHeader("Authorization"); header != "" {
		// A malformed Authorization header is rejected rather than ignored, so
		// a bad bearer token cannot silently fall back to a different cookie.
		scheme, token, found := strings.Cut(header, " ")
		token = strings.TrimSpace(token)
		if !found || !strings.EqualFold(scheme, "Bearer") || token == "" {
			return "", false
		}
		return token, true
	}

	if cookieName == "" {
		return "", false
	}
	cookie, err := c.Cookie(cookieName)
	if err != nil || cookie == "" {
		return "", false
	}
	return cookie, true
}
