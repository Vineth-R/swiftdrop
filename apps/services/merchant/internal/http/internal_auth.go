package servicehttp

import (
	"crypto/sha256"
	"crypto/subtle"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/Vineth-R/swiftdrop/packages/go-common/httpx"
)

func requireInternalToken(c *gin.Context, token string) bool {
	expected := sha256.Sum256([]byte(token))
	supplied := sha256.Sum256([]byte(c.GetHeader("X-Internal-Token")))
	if token == "" || c.GetHeader("X-Internal-Token") == "" || subtle.ConstantTimeCompare(expected[:], supplied[:]) != 1 {
		httpx.Error(c, http.StatusUnauthorized, "UNAUTHENTICATED", "authentication required")
		return false
	}
	return true
}
