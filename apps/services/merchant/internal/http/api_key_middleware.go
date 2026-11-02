package servicehttp

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/apikey"
	"github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/repository"
	"github.com/Vineth-R/swiftdrop/packages/go-common/httpx"
	"github.com/Vineth-R/swiftdrop/packages/go-common/requestid"
)

// MerchantAPIKey authenticates server-to-server requests without exposing the key.
func MerchantAPIKey(repo *repository.Repository, logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		provided := c.GetHeader("X-API-Key")
		lookupID, err := apikey.Parse(provided)
		if err != nil {
			httpx.Error(c, http.StatusUnauthorized, "INVALID_API_KEY", "invalid merchant API key")
			return
		}
		client, storedHash, err := repo.APIClientByLookup(c.Request.Context(), lookupID)
		if err != nil {
			writeAPIKeyError(c, logger, "lookup_api_key", err)
			return
		}
		if !apikey.EqualHash(storedHash, apikey.Hash(provided)) || client.Status != "ACTIVE" {
			httpx.Error(c, http.StatusUnauthorized, "INVALID_API_KEY", "invalid merchant API key")
			return
		}
		merchant, err := repo.MerchantByID(c.Request.Context(), client.MerchantID)
		if err != nil {
			writeAPIKeyError(c, logger, "load_merchant", err)
			return
		}
		if merchant.Status != repository.MerchantStatusApproved {
			httpx.Error(c, http.StatusForbidden, "MERCHANT_NOT_APPROVED", "merchant approval is required")
			return
		}
		_ = repo.TouchAPIClient(c.Request.Context(), client.ID)
		WithIdentity(c, Identity{MerchantID: client.MerchantID, Role: "MERCHANT"})
		c.Next()
	}
}

// writeAPIKeyError keeps unknown or revoked credentials distinct from a
// repository outage. Only the not-found case is an authentication failure.
func writeAPIKeyError(c *gin.Context, logger *slog.Logger, operation string, err error) {
	if errors.Is(err, repository.ErrNotFound) {
		httpx.Error(c, http.StatusUnauthorized, "INVALID_API_KEY", "invalid merchant API key")
		return
	}

	if logger != nil {
		logger.ErrorContext(c.Request.Context(), "merchant api key request failed",
			"request_id", requestid.FromGin(c),
			"path", c.FullPath(),
			"operation", operation,
			"error", err.Error(),
		)
	}
	httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
}
