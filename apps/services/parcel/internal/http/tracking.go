package servicehttp

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/Vineth-R/swiftdrop/apps/services/parcel/internal/parcels"
	"github.com/Vineth-R/swiftdrop/apps/services/parcel/internal/tracking"
	"github.com/Vineth-R/swiftdrop/packages/go-common/httpx"
	"github.com/Vineth-R/swiftdrop/packages/go-common/requestid"
	"github.com/gin-gonic/gin"
)

type TrackingStore interface {
	Lookup(context.Context, string) (tracking.Result, error)
}
type Options struct {
	Parcels        *parcels.Service
	InternalToken  string
	Tracking       TrackingStore
	Reports        ReportStore
	TrustedProxies []string
	AccessSecret   string
	CookieName     string
}

func registerTracking(router *gin.Engine, logger *slog.Logger, options Options) {
	store := options.Tracking
	if store == nil {
		store = tracking.NewRepository(nil)
	}
	router.GET("/tracking/:trackingNumber", httpx.NewRateLimiter(30, time.Minute).Middleware(), func(c *gin.Context) {
		result, err := store.Lookup(c.Request.Context(), c.Param("trackingNumber"))
		switch {
		case errors.Is(err, tracking.ErrInvalid):
			httpx.ErrorWithDetails(c, 422, "VALIDATION_ERROR", "invalid tracking number", []httpx.ErrorItem{{Field: "trackingNumber", Issue: "must be SDK or SDN followed by eight uppercase letters or digits"}})
		case errors.Is(err, tracking.ErrNotFound):
			httpx.Error(c, 404, "TRACKING_NOT_FOUND", "tracking number not found")
		case errors.Is(err, tracking.ErrUnavailable):
			httpx.Error(c, 503, "SERVICE_UNAVAILABLE", "tracking is unavailable")
		case err != nil:
			logger.ErrorContext(c.Request.Context(), "public tracking failed", "request_id", requestid.FromGin(c), "error", err)
			httpx.Error(c, 500, "INTERNAL_ERROR", "tracking lookup failed")
		default:
			c.JSON(200, gin.H{"data": result, "meta": gin.H{"requestId": requestid.FromGin(c), "timestamp": time.Now().UTC()}})
		}
	})
}
