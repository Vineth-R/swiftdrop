package httpx

import (
	"crypto/subtle"
	"net/http"

	"github.com/gin-gonic/gin"
)

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
