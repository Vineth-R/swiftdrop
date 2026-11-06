package servicehttp

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Vineth-R/swiftdrop/apps/services/parcel/internal/dispatch"
	"github.com/Vineth-R/swiftdrop/apps/services/parcel/internal/identityclient"
	"github.com/Vineth-R/swiftdrop/apps/services/parcel/internal/statemachine"
	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
	"github.com/Vineth-R/swiftdrop/packages/go-common/httpx"
	"github.com/Vineth-R/swiftdrop/packages/go-common/requestid"
	"github.com/Vineth-R/swiftdrop/packages/go-common/validation"
)

// DispatchService defines operations consumed by dispatch handlers.
type DispatchService interface {
	ListUnassigned(ctx context.Context, filter dispatch.UnassignedFilter) (dispatch.UnassignedPage, error)
	AssignCourier(ctx context.Context, input dispatch.AssignInput) (dispatch.AssignResult, error)
	GetWorkload(ctx context.Context, authHeader string, cookie *http.Cookie) ([]dispatch.CourierWorkload, error)
	ListAdminParcels(ctx context.Context, filter dispatch.AdminParcelFilter) (dispatch.AdminParcelPage, error)
}

type dispatchHandlers struct {
	service DispatchService
	logger  *slog.Logger
	cookie  string
}

func registerDispatch(router *gin.Engine, logger *slog.Logger, options Options) error {
	service := options.Dispatch
	if service == nil {
		service = dispatch.NewService(nil, nil)
	}

	cookie := options.CookieName
	if cookie == "" {
		cookie = "sd_access"
	}

	handlers := &dispatchHandlers{
		service: service,
		logger:  logger,
		cookie:  cookie,
	}

	dispatchGroup := router.Group("/dispatch", noStore())
	adminParcelsGroup := router.Group("/admin/parcels", noStore())

	if options.AccessSecret == "" {
		failClosed := func(c *gin.Context) {
			httpx.Error(c, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "authentication is not configured")
			c.Abort()
		}
		dispatchGroup.Use(failClosed)
		adminParcelsGroup.Use(failClosed)
	} else {
		manager, err := auth.NewTokenManager(options.AccessSecret, time.Minute)
		if err != nil {
			return err
		}
		authMiddleware := auth.Authenticate(manager, cookie)
		roleMiddleware := auth.RequireRoles(auth.RoleDispatcher)

		dispatchGroup.Use(authMiddleware, roleMiddleware)
		adminParcelsGroup.Use(authMiddleware, roleMiddleware)
	}

	// 5.1 GET /dispatch/unassigned
	dispatchGroup.GET("/unassigned", handlers.unassigned)

	// 5.2 PUT /dispatch/parcels/:parcelId/assignment
	dispatchGroup.PUT("/parcels/:parcelId/assignment", handlers.assignCourier)

	// 5.3 GET /dispatch/workload
	dispatchGroup.GET("/workload", handlers.workload)

	// Gateway and route probe support
	dispatchGroup.GET("/probe", handlers.probe)

	// 5.4 GET /admin/parcels
	adminParcelsGroup.GET("", handlers.adminParcels)
	adminParcelsGroup.GET("/", handlers.adminParcels)
	adminParcelsGroup.GET("/probe", handlers.probe)

	return nil
}

func (h *dispatchHandlers) probe(c *gin.Context) {
	httpx.Success(c, http.StatusOK, gin.H{"status": "ok"})
}

func (h *dispatchHandlers) unassigned(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))
	sort := c.DefaultQuery("sort", "createdAt:asc")

	filter := dispatch.UnassignedFilter{
		Page:     page,
		PageSize: pageSize,
		Sort:     sort,
	}

	if zoneID := c.Query("zoneId"); zoneID != "" {
		filter.ZoneID = &zoneID
	}
	if fromStr := c.Query("from"); fromStr != "" {
		if t, err := time.Parse(time.RFC3339, fromStr); err == nil {
			filter.From = &t
		}
	}
	if toStr := c.Query("to"); toStr != "" {
		if t, err := time.Parse(time.RFC3339, toStr); err == nil {
			filter.To = &t
		}
	}

	result, err := h.service.ListUnassigned(c.Request.Context(), filter)
	if err != nil {
		h.handleError(c, err)
		return
	}

	httpx.SuccessList(c, http.StatusOK, result.Items, httpx.NewPagination(result.Page, result.PageSize, result.TotalItems))
}

func (h *dispatchHandlers) assignCourier(c *gin.Context) {
	parcelID := c.Param("parcelId")

	var req struct {
		CourierID string `json:"courierId"`
		Reason    string `json:"reason"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	claims, ok := auth.ClaimsFrom(c)
	actorUserID := ""
	if ok {
		actorUserID = claims.UserID
	}

	result, err := h.service.AssignCourier(c.Request.Context(), dispatch.AssignInput{
		ParcelID:    parcelID,
		CourierID:   req.CourierID,
		Reason:      req.Reason,
		ActorUserID: actorUserID,
	})
	if err != nil {
		h.handleError(c, err)
		return
	}

	httpx.Success(c, http.StatusOK, result)
}

func (h *dispatchHandlers) workload(c *gin.Context) {
	authHeader := c.GetHeader("Authorization")
	cookie, _ := c.Request.Cookie(h.cookie)

	items, err := h.service.GetWorkload(c.Request.Context(), authHeader, cookie)
	if err != nil {
		h.handleError(c, err)
		return
	}

	httpx.Success(c, http.StatusOK, items)
}

func (h *dispatchHandlers) adminParcels(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))
	sort := c.DefaultQuery("sort", "createdAt:desc")

	filter := dispatch.AdminParcelFilter{
		Page:     page,
		PageSize: pageSize,
		Sort:     sort,
	}

	if status := c.Query("status"); status != "" {
		filter.Status = &status
	}
	if courierID := c.Query("courierId"); courierID != "" {
		filter.CourierID = &courierID
	}
	if merchantID := c.Query("merchantId"); merchantID != "" {
		filter.MerchantID = &merchantID
	}
	if customerID := c.Query("customerId"); customerID != "" {
		filter.CustomerID = &customerID
	}
	if trackingNumber := c.Query("trackingNumber"); trackingNumber != "" {
		filter.TrackingNumber = &trackingNumber
	}
	if fromStr := c.Query("from"); fromStr != "" {
		if t, err := time.Parse(time.RFC3339, fromStr); err == nil {
			filter.From = &t
		}
	}
	if toStr := c.Query("to"); toStr != "" {
		if t, err := time.Parse(time.RFC3339, toStr); err == nil {
			filter.To = &t
		}
	}

	result, err := h.service.ListAdminParcels(c.Request.Context(), filter)
	if err != nil {
		h.handleError(c, err)
		return
	}

	httpx.SuccessList(c, http.StatusOK, result.Items, httpx.NewPagination(result.Page, result.PageSize, result.TotalItems))
}

func (h *dispatchHandlers) handleError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, dispatch.ErrParcelNotFound):
		httpx.Error(c, http.StatusNotFound, "PARCEL_NOT_FOUND", "parcel not found")

	case errors.Is(err, dispatch.ErrParcelClosed), errors.Is(err, statemachine.ErrParcelClosed):
		httpx.Error(c, http.StatusConflict, "PARCEL_CLOSED", "closed parcels cannot be assigned or reassigned")

	case errors.Is(err, dispatch.ErrCourierNotEligible), errors.Is(err, identityclient.ErrCourierNotEligible):
		httpx.Error(c, http.StatusUnprocessableEntity, "COURIER_NOT_ELIGIBLE", "courier is not eligible for assignment")

	case errors.Is(err, dispatch.ErrInvalidInput):
		httpx.Error(c, http.StatusUnprocessableEntity, "VALIDATION_ERROR", err.Error())

	case errors.Is(err, dispatch.ErrServiceUnavailable), errors.Is(err, identityclient.ErrServiceUnavailable):
		httpx.Error(c, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "identity service is unavailable")

	default:
		h.logger.ErrorContext(c.Request.Context(), "dispatch operation failed",
			"request_id", requestid.FromGin(c),
			"error", err,
		)
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
	}
}

// Ensure validation helper is satisfied if needed
var _ = validation.UUID
