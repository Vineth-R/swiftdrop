package sessioncheck

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
	"github.com/Vineth-R/swiftdrop/packages/go-common/httpx"
)

// Middleware rejects a request whose verified claims no longer match identity.
// It must run after auth.Authenticate, which stores the claims. A checker that
// has not been configured fails closed with 503 rather than silently skipping
// the revocation check.
func Middleware(checker Guard) gin.HandlerFunc {
	return func(c *gin.Context) {
		claims, ok := auth.ClaimsFrom(c)
		if !ok {
			httpx.Error(c, http.StatusUnauthorized, "UNAUTHENTICATED", "authentication required")
			return
		}
		if checker == nil {
			httpx.Error(c, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "session state could not be verified")
			return
		}
		switch err := checker.Check(c.Request.Context(), claims); {
		case err == nil:
			c.Next()
		case errors.Is(err, ErrRevoked):
			httpx.Error(c, http.StatusUnauthorized, "SESSION_REVOKED", "this session is no longer valid")
		default:
			httpx.Error(c, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "session state could not be verified")
		}
	}
}
