package servicehttp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/Vineth-R/swiftdrop/apps/services/pricing/internal/pricing"
	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
	"github.com/Vineth-R/swiftdrop/packages/go-common/httpx"
	"github.com/Vineth-R/swiftdrop/packages/go-common/requestid"
	"github.com/gin-gonic/gin"
)

type PricingStore interface {
	Quote(context.Context, pricing.Input) (pricing.Quote, error)
	Zones(context.Context) ([]pricing.Zone, error)
	Zone(context.Context, string) (pricing.Zone, error)
	Mutate(context.Context, string, string, string, pricing.Mutation) (json.RawMessage, error)
	List(context.Context, string, pricing.Filter) ([]json.RawMessage, int, error)
}
type Options struct {
	Store          PricingStore
	InternalToken  string
	AccessSecret   string
	CookieName     string
	TrustedProxies []string
}
type pricingHandler struct {
	store  PricingStore
	logger *slog.Logger
}

func registerPricing(router *gin.Engine, logger *slog.Logger, options Options) error {
	store := options.Store
	if store == nil {
		store = pricing.NewRepository(nil)
	}
	h := &pricingHandler{store: store, logger: logger}
	router.POST("/pricing/estimate", httpx.NewRateLimiter(300, time.Minute).Middleware(), h.quote)
	router.GET("/zones", h.zones)
	internal := router.Group("/internal", httpx.InternalToken(options.InternalToken))
	internal.POST("/pricing/quote", h.quote)
	internal.GET("/zones/:zoneId", h.zone)
	cookie := options.CookieName
	if cookie == "" {
		cookie = "sd_access"
	}
	admin := router.Group("/admin")
	if options.AccessSecret == "" {
		admin.Use(func(c *gin.Context) {
			httpx.Error(c, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "authentication is not configured")
		})
	} else {
		manager, err := auth.NewTokenManager(options.AccessSecret, time.Minute)
		if err != nil {
			return err
		}
		admin.Use(auth.Authenticate(manager, cookie), auth.RequireRoles(auth.RoleAdmin))
	}
	admin.GET("/zones", h.list("zones"))
	admin.POST("/zones", h.mutate("zones", true))
	admin.PATCH("/zones/:id", h.mutate("zones", false))
	for _, kind := range []string{"rules", "bands"} {
		admin.GET("/pricing/"+kind, h.list(kind))
		admin.POST("/pricing/"+kind, h.mutate(kind, true))
		admin.PATCH("/pricing/"+kind+"/:id", h.mutate(kind, false))
	}
	admin.GET("/pricing/surcharges", h.list("surcharges"))
	admin.PUT("/pricing/surcharges/:id", h.mutate("surcharges", false))
	return nil
}

func decodeBody(c *gin.Context, value any) error {
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
		return errors.New("request must contain one JSON object")
	}
	return nil
}
func respond(c *gin.Context, status int, data any) {
	c.JSON(status, gin.H{"data": data, "meta": gin.H{"requestId": requestid.FromGin(c), "timestamp": time.Now().UTC()}})
}
func (h *pricingHandler) failure(c *gin.Context, err error) {
	var invalid *pricing.Invalid
	var conflict *pricing.Conflict
	switch {
	case errors.As(err, &invalid):
		httpx.ErrorWithDetails(c, 422, "VALIDATION_ERROR", "one or more fields are invalid", []httpx.ErrorItem{{Field: invalid.Field, Issue: invalid.Issue}})
	case errors.As(err, &conflict):
		httpx.Error(c, 409, conflict.Code, "pricing configuration conflicts with existing data")
	case errors.Is(err, pricing.ErrZoneNotFound):
		httpx.Error(c, 404, "ZONE_NOT_FOUND", "zone not found")
	case errors.Is(err, pricing.ErrNotFound):
		httpx.Error(c, 404, "NOT_FOUND", "pricing configuration not found")
	case errors.Is(err, pricing.ErrRouteNotServiceable):
		httpx.Error(c, 422, "ROUTE_NOT_SERVICEABLE", "route is not serviceable")
	case errors.Is(err, pricing.ErrWeightNotPriced):
		httpx.Error(c, 422, "WEIGHT_NOT_PRICED", "weight has no active price band")
	case errors.Is(err, pricing.ErrUnavailable):
		httpx.Error(c, 503, "SERVICE_UNAVAILABLE", "pricing is unavailable")
	default:
		h.logger.ErrorContext(c.Request.Context(), "pricing operation failed", "request_id", requestid.FromGin(c), "error", err)
		httpx.Error(c, 500, "INTERNAL_ERROR", "pricing operation failed")
	}
}
func (h *pricingHandler) quote(c *gin.Context) {
	var input pricing.Input
	if err := decodeBody(c, &input); err != nil {
		h.bodyError(c, err)
		return
	}
	if err := input.Validate(); err != nil {
		h.failure(c, err)
		return
	}
	quote, err := h.store.Quote(c.Request.Context(), input)
	if err != nil {
		h.failure(c, err)
		return
	}
	respond(c, 200, quote)
}
func (h *pricingHandler) zones(c *gin.Context) {
	zones, err := h.store.Zones(c.Request.Context())
	if err != nil {
		h.failure(c, err)
		return
	}
	public := make([]gin.H, 0, len(zones))
	for _, zone := range zones {
		public = append(public, gin.H{"id": zone.ID, "code": zone.Code, "name": zone.Name, "districts": zone.Districts})
	}
	respond(c, 200, public)
}
func (h *pricingHandler) zone(c *gin.Context) {
	zone, err := h.store.Zone(c.Request.Context(), c.Param("zoneId"))
	if err != nil {
		h.failure(c, err)
		return
	}
	respond(c, 200, gin.H{"id": zone.ID, "code": zone.Code, "name": zone.Name, "isActive": zone.IsActive})
}
func (h *pricingHandler) mutate(kind string, creating bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		if !creating && kind != "surcharges" && !pricing.ValidID(id) {
			h.failure(c, &pricing.Invalid{Field: "id", Issue: "must be a UUID"})
			return
		}
		if kind == "surcharges" && !pricing.ValidSize(id) {
			h.failure(c, &pricing.Invalid{Field: "sizeCategory", Issue: "unsupported category"})
			return
		}
		claims, ok := auth.ClaimsFrom(c)
		if !ok {
			httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
			return
		}
		actor := claims.UserID
		if !pricing.ValidID(actor) {
			httpx.Error(c, 401, "UNAUTHENTICATED", "invalid user identity")
			return
		}
		var fields map[string]json.RawMessage
		if err := decodeBody(c, &fields); err != nil {
			h.bodyError(c, err)
			return
		}
		if fields == nil {
			httpx.Error(c, 400, "BAD_REQUEST", "request must be an object")
			return
		}
		allowed := pricing.AllowedFields(kind, creating)
		for name, value := range fields {
			if !allowed[name] || string(value) == "null" {
				h.failure(c, &pricing.Invalid{Field: name, Issue: "field is unknown or null"})
				return
			}
		}
		raw, err := json.Marshal(fields)
		if err != nil {
			h.failure(c, err)
			return
		}
		var mutation pricing.Mutation
		if err := json.Unmarshal(raw, &mutation); err != nil {
			h.failure(c, &pricing.Invalid{Field: "request", Issue: "invalid field type or decimal precision"})
			return
		}
		if err := mutation.Validate(kind, creating); err != nil {
			h.failure(c, err)
			return
		}
		result, err := h.store.Mutate(c.Request.Context(), kind, id, actor, mutation)
		if err != nil {
			h.failure(c, err)
			return
		}
		status := 200
		if creating {
			status = 201
		}
		respond(c, status, result)
	}
}

func (h *pricingHandler) bodyError(c *gin.Context, err error) {
	var oversized *http.MaxBytesError
	var invalid *pricing.Invalid
	var wrongType *json.UnmarshalTypeError
	switch {
	case errors.As(err, &oversized):
		httpx.Error(c, 413, "PAYLOAD_TOO_LARGE", "request body exceeds 1 MiB")
	case errors.As(err, &invalid):
		h.failure(c, invalid)
	case errors.As(err, &wrongType):
		h.failure(c, &pricing.Invalid{Field: wrongType.Field, Issue: "incorrect field type"})
	default:
		httpx.Error(c, 400, "BAD_REQUEST", "invalid JSON pricing request")
	}
}
func (h *pricingHandler) list(kind string) gin.HandlerFunc {
	return func(c *gin.Context) {
		filter := pricing.Filter{Page: 1, PageSize: 20, Origin: c.Query("originZoneId"), Destination: c.Query("destinationZoneId")}
		for _, item := range []struct {
			key     string
			target  *int
			maximum int
		}{
			{key: "page", target: &filter.Page, maximum: 10000}, {key: "pageSize", target: &filter.PageSize, maximum: 100},
		} {
			if value, exists := c.GetQuery(item.key); exists {
				parsed, err := strconv.Atoi(value)
				if err != nil || parsed < 1 || parsed > item.maximum {
					h.failure(c, &pricing.Invalid{Field: item.key, Issue: "invalid pagination bound"})
					return
				}
				*item.target = parsed
			}
		}
		if kind == "zones" {
			for _, key := range []string{"originZoneId", "destinationZoneId"} {
				if _, exists := c.GetQuery(key); exists {
					h.failure(c, &pricing.Invalid{Field: key, Issue: "is not supported for zones"})
					return
				}
			}
		}
		for _, id := range []string{filter.Origin, filter.Destination} {
			if id != "" && !pricing.ValidID(id) {
				h.failure(c, &pricing.Invalid{Field: "zoneId", Issue: "must be a UUID"})
				return
			}
		}
		if value, exists := c.GetQuery("isActive"); exists {
			parsed, err := strconv.ParseBool(value)
			if err != nil {
				h.failure(c, &pricing.Invalid{Field: "isActive", Issue: "must be a boolean"})
				return
			}
			filter.Active = &parsed
		}
		filter.Sort = c.Query("sort")
		data, total, err := h.store.List(c.Request.Context(), kind, filter)
		if err != nil {
			h.failure(c, err)
			return
		}
		c.JSON(200, gin.H{"data": data, "meta": gin.H{"requestId": requestid.FromGin(c), "timestamp": time.Now().UTC(),
			"pagination": gin.H{"page": filter.Page, "pageSize": filter.PageSize, "totalItems": total, "totalPages": (total + filter.PageSize - 1) / filter.PageSize}}})
	}
}
