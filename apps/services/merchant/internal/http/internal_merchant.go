package servicehttp

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/repository"
	"github.com/Vineth-R/swiftdrop/packages/go-common/httpx"
	"github.com/Vineth-R/swiftdrop/packages/go-common/requestid"
	"github.com/Vineth-R/swiftdrop/packages/go-common/validation"
)

// registerInternalMerchantRoutes exposes the recipient lookup the notification
// worker uses for merchant-owned parcel events. The route is always registered
// so a missing repository fails closed with 503 instead of looking like an
// unknown path; the shared token is checked first.
func RegisterInternalMerchantRoutes(router *gin.Engine, repo *repository.Repository, token string, logger *slog.Logger) {
	group := router.Group("/internal/merchants",
		noStore(), httpx.InternalToken(token), requireInternalRequestID())
	if repo == nil {
		group.Use(unavailable("internal merchant contact lookup is not configured"))
	}
	group.GET("/:merchantId/contact", func(c *gin.Context) {
		if repo == nil {
			httpx.Error(c, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "internal merchant contact lookup is not configured")
			return
		}
		merchantID := c.Param("merchantId")
		if !validation.UUID(merchantID) {
			httpx.Error(c, http.StatusNotFound, "NOT_FOUND", "merchant not found")
			return
		}
		merchant, err := repo.MerchantByID(c.Request.Context(), merchantID)
		if errors.Is(err, repository.ErrNotFound) {
			httpx.Error(c, http.StatusNotFound, "NOT_FOUND", "merchant not found")
			return
		}
		if err != nil {
			if logger != nil {
				logger.ErrorContext(c.Request.Context(), "internal merchant contact lookup failed",
					"request_id", requestid.FromGin(c), "operation", "internal_merchant_contact")
			}
			httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "merchant contact lookup failed")
			return
		}
		httpx.Success(c, http.StatusOK, gin.H{
			"merchantId":   merchant.ID,
			"ownerUserId":  merchant.OwnerUserID,
			"businessName": merchant.BusinessName,
			"contactEmail": merchant.ContactEmail,
			"status":       merchant.Status,
		})
	})
}
