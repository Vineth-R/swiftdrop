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

const apiClientIDKey = "merchant.api_client_id"

// MerchantAPIKey authenticates server-to-server requests without exposing the key.
func MerchantAPIKey(repo *repository.Repository, logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		if authenticateAPIKey(c, repo, logger) {
			c.Next()
		}
	}
}

// authenticateAPIKey verifies the key and stores the identity without advancing
// the Gin chain. It lets the merchant order routes choose between an API key
// and a session without nesting c.Next() calls.
func authenticateAPIKey(c *gin.Context, repo *repository.Repository, logger *slog.Logger) bool {
	if repo == nil {
		httpx.Error(c, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "merchant persistence is not configured")
		return false
	}
	provided := c.GetHeader("X-API-Key")
	lookupID, err := apikey.Parse(provided)
	if err != nil {
		httpx.Error(c, http.StatusUnauthorized, "INVALID_API_KEY", "invalid merchant API key")
		return false
	}
	client, storedHash, err := repo.APIClientByLookup(c.Request.Context(), lookupID)
	if err != nil {
		writeAPIKeyError(c, logger, "lookup_api_key", err)
		return false
	}
	if !apikey.EqualHash(storedHash, apikey.Hash(provided)) || client.Status != "ACTIVE" {
		httpx.Error(c, http.StatusUnauthorized, "INVALID_API_KEY", "invalid merchant API key")
		return false
	}
	merchant, err := repo.MerchantByID(c.Request.Context(), client.MerchantID)
	if err != nil {
		writeAPIKeyError(c, logger, "load_merchant", err)
		return false
	}
	if merchant.Status != repository.MerchantStatusApproved {
		httpx.Error(c, http.StatusForbidden, "MERCHANT_NOT_APPROVED", "merchant approval is required")
		return false
	}
	_ = repo.TouchAPIClient(c.Request.Context(), client.ID)
	c.Set(apiClientIDKey, client.ID)
	// An API key acts for the merchant owner, so identityFrom can accept the
	// identity (it requires a non-empty UserID) without inventing a user.
	WithIdentity(c, Identity{UserID: merchant.OwnerUserID, MerchantID: client.MerchantID, Role: "MERCHANT"})
	return true
}

// apiClientIDFrom returns the authenticated key ID when one was used.
func apiClientIDFrom(c *gin.Context) string {
	value, ok := c.Get(apiClientIDKey)
	if !ok {
		return ""
	}
	id, _ := value.(string)
	return id
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
			"error_class", errorClass(err),
		)
	}
	httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
}
