package servicehttp

import (
	"context"
	"errors"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Vineth-R/swiftdrop/apps/services/parcel/internal/courier"
	"github.com/Vineth-R/swiftdrop/apps/services/parcel/internal/repository"
	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
	"github.com/Vineth-R/swiftdrop/packages/go-common/httpx"
	"github.com/Vineth-R/swiftdrop/packages/go-common/requestid"
	"github.com/Vineth-R/swiftdrop/packages/go-common/validation"
)

// CourierStore is the assignment-scoped read side consumed by these handlers.
type CourierStore interface {
	List(context.Context, courier.Filter) (courier.Page, error)
	Detail(context.Context, string, string) (courier.Detail, error)
}

type courierHandlers struct {
	store  CourierStore
	logger *slog.Logger
}

func registerCourier(router *gin.Engine, logger *slog.Logger, options Options) error {
	store := options.Courier
	if store == nil {
		store = courier.NewRepository(nil)
	}
	h := &courierHandlers{store: store, logger: logger}
	group := router.Group("/courier/deliveries", noStore())
	if options.AccessSecret == "" {
		group.Use(func(c *gin.Context) {
			httpx.Error(c, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "authentication is not configured")
		})
	} else {
		manager, err := auth.NewTokenManager(options.AccessSecret, time.Minute)
		if err != nil {
			return err
		}
		cookie := options.CookieName
		if cookie == "" {
			cookie = "sd_access"
		}
		group.Use(auth.Authenticate(manager, cookie), auth.RequireRoles(auth.RoleCourier))
	}
	// Register both forms so a trailing slash never falls back to Gin's redirect,
	// whose Location would omit the gateway's external /api/v1 prefix.
	group.GET("", h.list)
	group.GET("/", h.list)
	group.GET("/:parcelId", h.detail)
	group.GET("/:parcelId/", h.detail)
	return nil
}

func (h *courierHandlers) list(c *gin.Context) {
	claims, ok := courierClaims(c)
	if !ok {
		return
	}
	query, err := url.ParseQuery(c.Request.URL.RawQuery)
	if err != nil {
		httpx.Error(c, http.StatusBadRequest, "BAD_REQUEST", "malformed query parameters")
		return
	}
	issues := validation.New()
	for key, values := range query {
		switch key {
		case "date", "status", "page", "pageSize":
			if len(values) != 1 || values[0] == "" {
				issues.Add(key, "must have one non-empty value")
			}
		default:
			// Never accept a caller-selected courier or another undocumented filter.
			issues.Add("query", "contains an unsupported parameter")
		}
	}
	page, pageSize := parsePagination(c, issues, 1, 20)
	if int64(page-1) > math.MaxInt64/int64(pageSize) {
		issues.Add("page", "is too large")
	}
	start, end, dateErr := courier.DayBounds(query.Get("date"), time.Now())
	if dateErr != nil {
		issues.Add("date", "must be a valid YYYY-MM-DD date")
	}
	status := query.Get("status")
	switch repository.ParcelStatus(status) {
	case "", repository.ParcelStatusCreated, repository.ParcelStatusPickedUp,
		repository.ParcelStatusInTransit, repository.ParcelStatusOutForDelivery,
		repository.ParcelStatusDelivered, repository.ParcelStatusFailed:
	default:
		issues.Add("status", "must be a valid parcel status")
	}
	if !issues.Empty() {
		httpx.ErrorWithDetails(c, http.StatusUnprocessableEntity, "VALIDATION_ERROR",
			"One or more fields are invalid.", httpx.ErrorItems(issues.Issues()))
		return
	}
	result, err := h.store.List(c.Request.Context(), courier.Filter{
		CourierID: claims.UserID, Start: start, End: end, Status: status, Page: page, PageSize: pageSize,
	})
	if h.failed(c, "list", err) {
		return
	}
	if result.Items == nil {
		result.Items = []courier.Delivery{}
	}
	httpx.SuccessList(c, http.StatusOK, result.Items, httpx.NewPagination(page, pageSize, result.TotalItems))
}

func (h *courierHandlers) detail(c *gin.Context) {
	claims, ok := courierClaims(c)
	if !ok {
		return
	}
	parcelID := c.Param("parcelId")
	if !validation.UUID(parcelID) {
		httpx.Error(c, http.StatusBadRequest, "BAD_REQUEST", "parcelId must be a valid UUID")
		return
	}
	result, err := h.store.Detail(c.Request.Context(), claims.UserID, parcelID)
	if h.failed(c, "detail", err) {
		return
	}
	httpx.Success(c, http.StatusOK, result)
}

func courierClaims(c *gin.Context) (auth.Claims, bool) {
	claims, ok := auth.ClaimsFrom(c)
	if !ok || !validation.UUID(claims.UserID) {
		httpx.Error(c, http.StatusUnauthorized, "UNAUTHENTICATED", "authentication required")
		return auth.Claims{}, false
	}
	claims.UserID = strings.ToLower(claims.UserID)
	return claims, true
}

func (h *courierHandlers) failed(c *gin.Context, operation string, err error) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, courier.ErrNotFound):
		httpx.Error(c, http.StatusNotFound, "PARCEL_NOT_FOUND", "parcel not found")
	case errors.Is(err, courier.ErrForbidden):
		httpx.Error(c, http.StatusForbidden, "FORBIDDEN", "parcel is not assigned to this courier")
	case errors.Is(err, courier.ErrUnavailable):
		httpx.Error(c, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "courier deliveries are unavailable")
	default:
		// Driver diagnostics can contain SQL and contact information. Log only a
		// stable operation and request ID, never the parcel or database error text.
		h.logger.ErrorContext(c.Request.Context(), "courier read failed",
			"request_id", requestid.FromGin(c), "operation", operation)
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "delivery information could not be loaded")
	}
	return true
}
