package middleware

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/Vineth-R/swiftdrop/apps/services/ai-assistant/internal/contract"
	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
	"github.com/Vineth-R/swiftdrop/packages/go-common/httpx"
)

const callerContextKey = "ai.caller"

// OptionalAuth attaches verified caller identity when credentials are present.
// Missing credentials leave the request anonymous. Supplied but invalid
// credentials are rejected so a stale cookie never silently downgrades a
// signed-in user to anonymous access.
func OptionalAuth(verifier auth.Verifier, cookieName string) gin.HandlerFunc {
	return func(c *gin.Context) {
		token, supplied, malformed := extractToken(c, cookieName)
		if !supplied {
			c.Set(callerContextKey, contract.CallerContext{})
			c.Next()
			return
		}
		if malformed {
			httpx.Error(c, http.StatusUnauthorized, "UNAUTHENTICATED", "authentication required")
			return
		}
		if verifier == nil {
			httpx.Error(c, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "authentication is not configured")
			return
		}

		claims, err := verifier.Verify(token)
		if err != nil {
			if errors.Is(err, auth.ErrTokenExpired) {
				httpx.Error(c, http.StatusUnauthorized, "TOKEN_EXPIRED", "access token expired")
				return
			}
			httpx.Error(c, http.StatusUnauthorized, "UNAUTHENTICATED", "authentication required")
			return
		}

		c.Set(callerContextKey, contract.CallerContext{
			UserID:        claims.UserID,
			Role:          string(claims.Role),
			Authenticated: true,
			AccessToken:   token,
		})
		c.Next()
	}
}

// CallerFrom returns the caller stored by OptionalAuth. The zero value is an
// anonymous caller, so callers must check Authenticated before private work.
func CallerFrom(c *gin.Context) contract.CallerContext {
	value, exists := c.Get(callerContextKey)
	if !exists {
		return contract.CallerContext{}
	}
	caller, ok := value.(contract.CallerContext)
	if !ok {
		return contract.CallerContext{}
	}
	return caller
}

func extractToken(c *gin.Context, cookieName string) (token string, supplied bool, malformed bool) {
	if header := c.GetHeader("Authorization"); header != "" {
		scheme, value, found := strings.Cut(header, " ")
		value = strings.TrimSpace(value)
		if !found || !strings.EqualFold(scheme, "Bearer") || value == "" {
			return "", true, true
		}
		return value, true, false
	}
	if cookieName == "" {
		return "", false, false
	}
	cookie, err := c.Cookie(cookieName)
	if err != nil || cookie == "" {
		return "", false, false
	}
	return cookie, true, false
}
