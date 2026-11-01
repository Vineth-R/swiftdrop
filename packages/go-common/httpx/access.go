package httpx

import (
	"crypto/subtle"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

type accessClaims struct {
	Role string `json:"role"`
	jwt.RegisteredClaims
}

// InternalToken refuses access when the service secret is not configured.
func InternalToken(secret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		provided := c.GetHeader("X-Internal-Token")
		if secret == "" || subtle.ConstantTimeCompare([]byte(provided), []byte(secret)) != 1 {
			Error(c, http.StatusUnauthorized, "UNAUTHENTICATED", "internal authentication required")
			return
		}
		c.Next()
	}
}

// RequireRole verifies access cookies or Bearer tokens; it never trusts role headers.
func RequireRole(secret, cookieName, role string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if secret == "" {
			Error(c, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "authentication is not configured")
			return
		}
		raw, cookieErr := c.Cookie(cookieName)
		if cookieErr != nil {
			raw = ""
		}
		if header := c.GetHeader("Authorization"); header != "" {
			parts := strings.Fields(header)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
				Error(c, http.StatusUnauthorized, "UNAUTHENTICATED", "authentication required")
				return
			}
			raw = parts[1]
		}
		claims := &accessClaims{}
		token, err := jwt.ParseWithClaims(raw, claims, func(token *jwt.Token) (any, error) {
			return []byte(secret), nil
		}, jwt.WithValidMethods([]string{"HS256"}), jwt.WithExpirationRequired(), jwt.WithIssuedAt())
		if errors.Is(err, jwt.ErrTokenExpired) {
			Error(c, 401, "TOKEN_EXPIRED", "access token expired")
			return
		}
		if err != nil || !token.Valid || claims.Subject == "" || claims.ID == "" || claims.IssuedAt == nil {
			Error(c, http.StatusUnauthorized, "UNAUTHENTICATED", "authentication required")
			return
		}
		if claims.Role != role {
			Error(c, http.StatusForbidden, "FORBIDDEN", "insufficient permissions")
			return
		}
		c.Set("authenticated_subject", claims.Subject)
		c.Next()
	}
}
