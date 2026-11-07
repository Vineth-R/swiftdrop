package servicehttp

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/Vineth-R/swiftdrop/apps/services/parcel/internal/parcels"
	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
	"github.com/Vineth-R/swiftdrop/packages/go-common/httpx"
	"github.com/Vineth-R/swiftdrop/packages/go-common/requestid"
	"github.com/gin-gonic/gin"
)

type parcelHandlers struct {
	service *parcels.Service
	logger  *slog.Logger
}

func registerParcels(router *gin.Engine, logger *slog.Logger, options Options) error {
	service := options.Parcels
	if service == nil {
		service = parcels.NewService(nil, nil)
	}
	h := &parcelHandlers{service, logger}
	group := router.Group("/parcels", noStore())
	if options.AccessSecret == "" {
		group.Use(func(c *gin.Context) { httpx.Error(c, 503, "SERVICE_UNAVAILABLE", "authentication is not configured") })
	} else {
		manager, err := auth.NewTokenManager(options.AccessSecret, time.Minute)
		if err != nil {
			return err
		}
		cookie := options.CookieName
		if cookie == "" {
			cookie = "sd_access"
		}
		group.Use(auth.Authenticate(manager, cookie))
	}
	group.POST("", auth.RequireRoles(auth.RoleCustomer), h.create)
	group.GET("", auth.RequireRoles(auth.RoleCustomer), h.list)
	group.GET("/:parcelId", auth.RequireRoles(auth.RoleCustomer, auth.RoleDispatcher), h.detail)
	group.GET("/:parcelId/history", auth.RequireRoles(auth.RoleCustomer, auth.RoleDispatcher), h.history)
	group.GET("/:parcelId/receipt", auth.RequireRoles(auth.RoleCustomer, auth.RoleAdmin), h.receipt)
	internal := router.Group("/internal/parcels", noStore(), httpx.InternalToken(options.InternalToken))
	internal.POST("", h.merchant)
	internal.GET("/:parcelId/summary", h.summary)
	return nil
}
func parcelBody(c *gin.Context, value any) error {
	if c.ContentType() != "application/json" {
		return errors.New("request must be JSON")
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1<<20)
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("request must contain one object")
	}
	return nil
}
func (h *parcelHandlers) body(c *gin.Context, value any) bool {
	if err := parcelBody(c, value); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			httpx.Error(c, 413, "PAYLOAD_TOO_LARGE", "request is too large")
		} else {
			httpx.Error(c, 400, "BAD_REQUEST", "request must contain valid JSON fields and decimal numbers")
		}
		return false
	}
	return true
}
func (h *parcelHandlers) failure(c *gin.Context, err error) bool {
	if err == nil {
		return false
	}
	var invalid *parcels.Invalid
	var changed *parcels.PriceChanged
	var pricing *parcels.PricingError
	switch {
	case errors.As(err, &invalid):
		httpx.ErrorWithDetails(c, 422, "VALIDATION_ERROR", "one or more fields are invalid", httpx.ErrorItems(invalid.Issues))
	case errors.As(err, &changed):
		c.AbortWithStatusJSON(409, gin.H{"error": gin.H{"code": "PRICE_CHANGED", "message": "the delivery fee has changed; confirm the current fee", "currentFee": changed.CurrentFee.Total}, "meta": gin.H{"requestId": requestid.FromGin(c)}})
	case errors.As(err, &pricing):
		httpx.Error(c, 422, pricing.Code, "the selected parcel cannot be priced")
	case errors.Is(err, parcels.ErrConflict):
		httpx.Error(c, 409, "IDEMPOTENCY_CONFLICT", "the key was already used for a different parcel")
	case errors.Is(err, parcels.ErrNotFound):
		httpx.Error(c, 404, "NOT_FOUND", "parcel not found")
	case errors.Is(err, parcels.ErrUnavailable):
		httpx.Error(c, 503, "SERVICE_UNAVAILABLE", "parcel service dependencies are unavailable")
	default:
		h.logger.ErrorContext(c.Request.Context(), "parcel operation failed",
			"request_id", requestid.FromGin(c),
			"path", c.FullPath(),
			"method", c.Request.Method,
			"error", err,
		)
		httpx.Error(c, 500, "INTERNAL_ERROR", "the parcel operation could not be completed")
	}
	return true
}
func (h *parcelHandlers) create(c *gin.Context) {
	var input parcels.CreateInput
	if !h.body(c, &input) {
		return
	}
	claims, ok := auth.ClaimsFrom(c)
	if !ok {
		httpx.Error(c, 401, "UNAUTHENTICATED", "authentication required")
		return
	}
	result, err := h.service.CreateCustomer(c.Request.Context(), claims.UserID, c.GetHeader("Idempotency-Key"), input)
	if !h.failure(c, err) {
		httpx.Success(c, 201, gin.H{"id": result.ID, "trackingNumber": result.TrackingNumber, "status": result.Status, "sender": gin.H{"name": result.Sender.Name, "city": result.Sender.City}, "recipient": gin.H{"name": result.Recipient.Name, "city": result.Recipient.City}, "weightKg": result.WeightKG, "sizeCategory": result.SizeCategory, "description": result.Description, "fee": result.Fee, "createdAt": result.CreatedAt})
	}
}
func parcelViewer(c *gin.Context) (string, bool) {
	claims, ok := auth.ClaimsFrom(c)
	if !ok {
		return "", false
	}
	return claims.UserID, claims.Role == auth.RoleAdmin || claims.Role == auth.RoleDispatcher
}
func (h *parcelHandlers) detail(c *gin.Context) {
	user, staff := parcelViewer(c)
	result, err := h.service.Detail(c.Request.Context(), c.Param("parcelId"), user, staff)
	if !h.failure(c, err) {
		httpx.Success(c, 200, result)
	}
}
func (h *parcelHandlers) history(c *gin.Context) {
	user, staff := parcelViewer(c)
	result, err := h.service.History(c.Request.Context(), c.Param("parcelId"), user, staff)
	if !h.failure(c, err) {
		httpx.Success(c, 200, result)
	}
}
func (h *parcelHandlers) receipt(c *gin.Context) {
	user, staff := parcelViewer(c)
	result, err := h.service.ReceiptData(c.Request.Context(), c.Param("parcelId"), user, staff)
	if h.failure(c, err) {
		return
	}
	pdf, err := parcels.Receipt(result)
	if err != nil {
		h.logger.ErrorContext(c.Request.Context(), "parcel receipt generation failed", "request_id", requestid.FromGin(c), "parcel_id", result.ID, "error", err)
		httpx.Error(c, 500, "INTERNAL_ERROR", "the receipt could not be generated")
		return
	}
	c.Header("Content-Disposition", `attachment; filename="receipt_`+result.TrackingNumber+`.pdf"`)
	c.Header("X-Content-Type-Options", "nosniff")
	c.Data(200, "application/pdf", pdf)
}
func (h *parcelHandlers) list(c *gin.Context) {
	f := parcels.Filter{Status: c.Query("status"), Sort: c.DefaultQuery("sort", "createdAt:desc"), Page: 1, PageSize: 20}
	for _, item := range []struct {
		key    string
		target *int
	}{{"page", &f.Page}, {"pageSize", &f.PageSize}} {
		if raw, ok := c.GetQuery(item.key); ok {
			n, err := strconv.Atoi(raw)
			if err != nil {
				httpx.ErrorWithDetails(c, 422, "VALIDATION_ERROR", "invalid pagination", []httpx.ErrorItem{{Field: item.key, Issue: "must be a positive integer"}})
				return
			}
			*item.target = n
		}
	}
	for _, item := range []struct {
		key    string
		target **time.Time
	}{{"from", &f.From}, {"to", &f.To}} {
		if raw, ok := c.GetQuery(item.key); ok {
			date, err := time.Parse("2006-01-02", raw)
			if err != nil {
				httpx.ErrorWithDetails(c, 422, "VALIDATION_ERROR", "invalid date", []httpx.ErrorItem{{Field: item.key, Issue: "must be an ISO date"}})
				return
			}
			*item.target = &date
		}
	}
	user, _ := parcelViewer(c)
	rows, total, err := h.service.List(c.Request.Context(), user, f)
	if !h.failure(c, err) {
		httpx.SuccessList(c, 200, rows, httpx.NewPagination(f.Page, f.PageSize, total))
	}
}
func (h *parcelHandlers) merchant(c *gin.Context) {
	var input parcels.MerchantInput
	if !h.body(c, &input) {
		return
	}
	result, err := h.service.CreateMerchant(c.Request.Context(), input)
	if !h.failure(c, err) {
		httpx.Success(c, 201, gin.H{"parcelId": result.ID, "trackingNumber": result.TrackingNumber, "status": result.Status, "fee": result.Fee})
	}
}
func (h *parcelHandlers) summary(c *gin.Context) {
	result, err := h.service.Summary(c.Request.Context(), c.Param("parcelId"))
	if !h.failure(c, err) {
		httpx.Success(c, 200, result)
	}
}
