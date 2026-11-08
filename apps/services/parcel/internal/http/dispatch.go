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
	issues := validation.New()
	page, pageSize := parsePagination(c, issues, 1, 20)
	from, to := parseTimeRange(c, issues)

	filter := dispatch.UnassignedFilter{
		Page:     page,
		PageSize: pageSize,
		Sort:     c.DefaultQuery("sort", "createdAt:asc"),
		From:     from,
		To:       to,
	}

	if zoneID := c.Query("zoneId"); zoneID != "" {
		if !validation.UUID(zoneID) {
			issues.Add("zoneId", "must be a valid UUID")
		} else {
			filter.ZoneID = &zoneID
		}
	}

	if !issues.Empty() {
		httpx.ErrorWithDetails(c, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "One or more parameters are invalid", httpx.ErrorItems(issues.Issues()))
		return
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

	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16<<10)
	var req struct {
		CourierID string `json:"courierId"`
		Reason    string `json:"reason"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			httpx.Error(c, http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE", "request body is too large")
			return
		}
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
	issues := validation.New()
	page, pageSize := parsePagination(c, issues, 1, 20)
	from, to := parseTimeRange(c, issues)

	filter := dispatch.AdminParcelFilter{
		Page:     page,
		PageSize: pageSize,
		Sort:     c.DefaultQuery("sort", "createdAt:desc"),
		From:     from,
		To:       to,
	}

	if status := c.Query("status"); status != "" {
		filter.Status = &status
	}
	if courierID := c.Query("courierId"); courierID != "" {
		if !validation.UUID(courierID) {
			issues.Add("courierId", "must be a valid UUID")
		} else {
			filter.CourierID = &courierID
		}
	}
	if merchantID := c.Query("merchantId"); merchantID != "" {
		if !validation.UUID(merchantID) {
			issues.Add("merchantId", "must be a valid UUID")
		} else {
			filter.MerchantID = &merchantID
		}
	}
	if customerID := c.Query("customerId"); customerID != "" {
		if !validation.UUID(customerID) {
			issues.Add("customerId", "must be a valid UUID")
		} else {
			filter.CustomerID = &customerID
		}
	}
	if trackingNumber := c.Query("trackingNumber"); trackingNumber != "" {
		filter.TrackingNumber = &trackingNumber
	}

	if !issues.Empty() {
		httpx.ErrorWithDetails(c, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "One or more parameters are invalid", httpx.ErrorItems(issues.Issues()))
		return
	}

	result, err := h.service.ListAdminParcels(c.Request.Context(), filter)
	if err != nil {
		h.handleError(c, err)
		return
	}

	httpx.SuccessList(c, http.StatusOK, result.Items, httpx.NewPagination(result.Page, result.PageSize, result.TotalItems))
}

func parsePagination(c *gin.Context, issues *validation.Collector, defaultPage, defaultPageSize int) (int, int) {
	page := defaultPage
	if pageStr := c.Query("page"); pageStr != "" {
		p, err := strconv.Atoi(pageStr)
		if err != nil || p < 1 {
			issues.Add("page", "must be a positive integer")
		} else {
			page = p
		}
	}

	pageSize := defaultPageSize
	if pageSizeStr := c.Query("pageSize"); pageSizeStr != "" {
		ps, err := strconv.Atoi(pageSizeStr)
		if err != nil || ps < 1 || ps > 100 {
			issues.Add("pageSize", "must be between 1 and 100")
		} else {
			pageSize = ps
		}
	}

	return page, pageSize
}

func parseTimeRange(c *gin.Context, issues *validation.Collector) (*time.Time, *time.Time) {
	var from, to *time.Time
	if fromStr := c.Query("from"); fromStr != "" {
		t, err := time.Parse(time.RFC3339, fromStr)
		if err != nil {
			issues.Add("from", "must be an RFC3339 date-time")
		} else {
			from = &t
		}
	}
	if toStr := c.Query("to"); toStr != "" {
		t, err := time.Parse(time.RFC3339, toStr)
		if err != nil {
			issues.Add("to", "must be an RFC3339 date-time")
		} else {
			to = &t
		}
	}
	if from != nil && to != nil && from.After(*to) {
		issues.Add("to", "must be after from")
	}
	return from, to
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
