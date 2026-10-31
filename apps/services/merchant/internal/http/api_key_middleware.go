package servicehttp

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/apikey"
	"github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/repository"
	"github.com/Vineth-R/swiftdrop/packages/go-common/httpx"
)

// MerchantAPIKey authenticates server-to-server requests without exposing the key.
func MerchantAPIKey(repo *repository.Repository) gin.HandlerFunc {
	return func(c *gin.Context) {
		provided := c.GetHeader("X-API-Key")
		lookupID, err := apikey.Parse(provided)
		if err != nil {
			httpx.Error(c, http.StatusUnauthorized, "invalid_api_key", "invalid merchant API key")
			return
		}
		client, storedHash, err := repo.APIClientByLookup(c.Request.Context(), lookupID)
		if errors.Is(err, repository.ErrNotFound) || err != nil {
			httpx.Error(c, http.StatusUnauthorized, "invalid_api_key", "invalid merchant API key")
			return
		}
		if !apikey.EqualHash(storedHash, apikey.Hash(provided)) || client.Status != "ACTIVE" {
			httpx.Error(c, http.StatusUnauthorized, "invalid_api_key", "invalid merchant API key")
			return
		}
		merchant, err := repo.MerchantByID(c.Request.Context(), client.MerchantID)
		if err != nil {
			httpx.Error(c, http.StatusUnauthorized, "invalid_api_key", "invalid merchant API key")
			return
		}
		if merchant.Status != repository.MerchantStatusApproved {
			httpx.Error(c, http.StatusForbidden, "merchant_not_approved", "merchant approval is required")
			return
		}
		_ = repo.TouchAPIClient(c.Request.Context(), client.ID)
		WithIdentity(c, Identity{MerchantID: client.MerchantID, Role: "MERCHANT"})
		c.Next()
	}
}
