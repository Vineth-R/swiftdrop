package servicehttp

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/Vineth-R/swiftdrop/apps/services/parcel/internal/parcels"
	"github.com/Vineth-R/swiftdrop/apps/services/parcel/internal/tracking"
	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
	"github.com/Vineth-R/swiftdrop/packages/go-common/httpx"
	"github.com/Vineth-R/swiftdrop/packages/go-common/requestid"
	"github.com/gin-gonic/gin"
)

type TrackingStore interface {
	Lookup(context.Context, string) (tracking.Result, error)
	LookupAuthorized(context.Context, string, string, bool) (tracking.Result, error)
}
type Options struct {
	Courier        CourierStore
	CourierStatus  CourierStatusStore
	CourierProof   CourierProofStore
	Parcels        *parcels.Service
	InternalToken  string
	Tracking       TrackingStore
	Reports        ReportStore
	Dispatch       DispatchService
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

	// Authenticated tracking is ownership-scoped. A customer only matches their
	// own parcel; DISPATCHER/ADMIN may look up any parcel operationally. The
	// public projection above is never used by the AI assistant.
	authorized := router.Group("/parcels/tracking", noStore(), httpx.NewRateLimiter(30, time.Minute).Middleware())
	if options.AccessSecret == "" {
		authorized.Use(func(c *gin.Context) {
			httpx.Error(c, 503, "SERVICE_UNAVAILABLE", "authentication is not configured")
			c.Abort()
		})
	} else {
		manager, err := auth.NewTokenManager(options.AccessSecret, time.Minute)
		if err != nil {
			logger.Error("parcel tracking token manager configuration failed", "error", err)
			authorized.Use(func(c *gin.Context) {
				httpx.Error(c, 503, "SERVICE_UNAVAILABLE", "authentication is not configured")
				c.Abort()
			})
		} else {
			cookie := options.CookieName
			if cookie == "" {
				cookie = "sd_access"
			}
			authorized.Use(auth.Authenticate(manager, cookie), auth.RequireRoles(auth.RoleCustomer, auth.RoleDispatcher, auth.RoleAdmin))
		}
	}
	authorized.GET("/:trackingNumber", func(c *gin.Context) {
		claims, ok := auth.ClaimsFrom(c)
		if !ok {
			httpx.Error(c, 401, "UNAUTHENTICATED", "authentication required")
			return
		}
		staff := claims.Role == auth.RoleDispatcher || claims.Role == auth.RoleAdmin
		result, err := store.LookupAuthorized(c.Request.Context(), c.Param("trackingNumber"), claims.UserID, staff)
		switch {
		case errors.Is(err, tracking.ErrInvalid):
			httpx.ErrorWithDetails(c, 422, "VALIDATION_ERROR", "invalid tracking number", []httpx.ErrorItem{{Field: "trackingNumber", Issue: "must be SDK or SDN followed by eight uppercase letters or digits"}})
		case errors.Is(err, tracking.ErrNotFound):
			// The same response covers a missing parcel and another customer's
			// parcel, so tracking numbers cannot be enumerated.
			httpx.Error(c, 404, "NOT_FOUND", "tracking number not found for this account")
		case errors.Is(err, tracking.ErrUnavailable):
			httpx.Error(c, 503, "SERVICE_UNAVAILABLE", "tracking is unavailable")
		case err != nil:
			logger.ErrorContext(c.Request.Context(), "authorized tracking failed", "request_id", requestid.FromGin(c), "error", err)
			httpx.Error(c, 500, "INTERNAL_ERROR", "tracking lookup failed")
		default:
			c.JSON(200, gin.H{"data": result, "meta": gin.H{"requestId": requestid.FromGin(c), "timestamp": time.Now().UTC()}})
		}
	})
}
