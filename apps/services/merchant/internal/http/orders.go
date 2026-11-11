package servicehttp

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/orders"
	"github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/repository"
	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
	"github.com/Vineth-R/swiftdrop/packages/go-common/httpx"
	"github.com/Vineth-R/swiftdrop/packages/go-common/validation"
)

func registerMerchantOrderRoutes(router *gin.Engine, h *merchantHandler, manager *auth.TokenManager) {
	if h.repository == nil {
		return
	}
	group := router.Group("/merchant/orders", noStore())
	limiter := httpx.NewRateLimiter(120, time.Minute)
	group.POST("",
		MerchantAPIKey(h.repository, h.logger),
		limiter.MiddlewareKeyed(func(c *gin.Context) string {
			if id := apiClientIDFrom(c); id != "" {
				return "key:" + id
			}
			if identity, ok := identityFrom(c); ok {
				return "merchant:" + identity.MerchantID
			}
			return ""
		}),
		h.createOrder,
	)
	group.GET("", h.orderAuth(manager), h.listOrders)
	group.GET("/:orderId", h.orderAuth(manager), h.getOrder)
}

// orderAuth accepts either an X-API-Key or a MERCHANT session. It avoids
// nesting c.Next(): the chosen branch sets the identity and returns, then this
// middleware advances once.
func (h *merchantHandler) orderAuth(manager *auth.TokenManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.GetHeader("X-API-Key") != "" {
			if !authenticateAPIKey(c, h.repository, h.logger) {
				return
			}
		} else {
			if manager == nil {
				httpx.Error(c, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "authentication is not configured")
				return
			}
			if !auth.AuthenticateRequest(c, manager, h.cookieName) {
				return
			}
			claims, ok := auth.ClaimsFrom(c)
			if !ok {
				httpx.Error(c, http.StatusUnauthorized, "UNAUTHENTICATED", "authentication required")
				return
			}
			if !claims.Role.Satisfies(auth.RoleMerchant) {
				httpx.Error(c, http.StatusForbidden, "FORBIDDEN", "not allowed")
				return
			}
			if !h.requireSessionCore(c) {
				return
			}
		}
		c.Next()
	}
}

func (h *merchantHandler) createOrder(c *gin.Context) {
	if h.orders == nil {
		httpx.Error(c, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "merchant orders are not configured")
		return
	}
	_, merchant, ok := h.merchantIdentity(c)
	if !ok || !h.requireApproved(c, merchant) {
		return
	}
	if c.ContentType() != "application/json" {
		httpx.Error(c, http.StatusUnsupportedMediaType, "UNSUPPORTED_MEDIA_TYPE", "content type must be application/json")
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 64<<10)
	var input orders.CreateInput
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		httpx.Error(c, http.StatusBadRequest, "BAD_REQUEST", "request body must contain valid merchant order fields")
		return
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		httpx.Error(c, http.StatusBadRequest, "BAD_REQUEST", "request body must contain one JSON object")
		return
	}
	result, err := h.orders.Create(c.Request.Context(), merchantProfile(merchant), c.GetHeader("Idempotency-Key"), input)
	if err != nil {
		h.orderError(c, err)
		return
	}
	status := http.StatusCreated
	if !result.Created {
		status = http.StatusOK
	}
	httpx.Success(c, status, orderCreateResponse(result.Order))
}

func (h *merchantHandler) listOrders(c *gin.Context) {
	if h.orders == nil {
		httpx.Error(c, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "merchant orders are not configured")
		return
	}
	identity, merchant, ok := h.merchantIdentity(c)
	if !ok || !h.requireApproved(c, merchant) {
		return
	}
	filter, issues := parseOrderList(c, identity.MerchantID)
	if len(issues) > 0 {
		httpx.ErrorWithDetails(c, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "one or more query parameters are invalid", httpx.ErrorItems(issues))
		return
	}
	items, total, err := h.orders.List(c.Request.Context(), filter)
	if err != nil {
		h.orderError(c, err)
		return
	}
	data := make([]merchantOrderItem, 0, len(items))
	for _, order := range items {
		data = append(data, orderItem(order))
	}
	httpx.SuccessList(c, http.StatusOK, data, httpx.NewPagination(filter.Page, filter.PageSize, total))
}

func (h *merchantHandler) getOrder(c *gin.Context) {
	if h.orders == nil {
		httpx.Error(c, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "merchant orders are not configured")
		return
	}
	identity, merchant, ok := h.merchantIdentity(c)
	if !ok || !h.requireApproved(c, merchant) {
		return
	}
	orderID := strings.ToLower(strings.TrimSpace(c.Param("orderId")))
	if !validation.UUID(orderID) {
		httpx.Error(c, http.StatusNotFound, "NOT_FOUND", "order not found")
		return
	}
	detail, err := h.orders.Detail(c.Request.Context(), identity.MerchantID, orderID)
	if err != nil {
		h.orderError(c, err)
		return
	}
	httpx.Success(c, http.StatusOK, orderDetailResponse(detail))
}

func (h *merchantHandler) orderError(c *gin.Context, err error) {
	var invalid *orders.Invalid
	var upstream *orders.UpstreamInvalid
	switch {
	case errors.As(err, &invalid):
		httpx.ErrorWithDetails(c, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "one or more fields are invalid", httpx.ErrorItems(invalid.Issues))
	case errors.As(err, &upstream):
		code := upstream.Code
		message := "the selected parcel cannot be priced"
		if code == "" || code == "VALIDATION_ERROR" {
			code = "VALIDATION_ERROR"
			message = "one or more fields are invalid"
		}
		if code != "VALIDATION_ERROR" && code != "ROUTE_NOT_SERVICEABLE" && code != "WEIGHT_NOT_PRICED" {
			code = "VALIDATION_ERROR"
			message = "one or more fields are invalid"
		}
		httpx.ErrorWithDetails(c, http.StatusUnprocessableEntity, code, message, httpx.ErrorItems(upstream.Issues))
	case errors.Is(err, orders.ErrDuplicateOrder):
		httpx.Error(c, http.StatusConflict, "DUPLICATE_ORDER", "an order with this reference or idempotency key already exists with a different payload")
	case errors.Is(err, orders.ErrNotFound):
		httpx.Error(c, http.StatusNotFound, "NOT_FOUND", "order not found")
	case errors.Is(err, orders.ErrZoneUnavailable):
		httpx.Error(c, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "the merchant pickup zone is unavailable")
	case errors.Is(err, orders.ErrParcelUnavailable):
		httpx.Error(c, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "parcel service is unavailable")
	default:
		h.internalError(c, "merchant_order", "the merchant order operation could not be completed", err)
	}
}

func merchantProfile(merchant repository.Merchant) orders.MerchantProfile {
	return orders.MerchantProfile{
		ID: merchant.ID, OwnerUserID: merchant.OwnerUserID, BusinessName: merchant.BusinessName,
		ContactPhone: merchant.ContactPhone, PickupAddress: merchant.PickupAddress,
		PickupZoneID: merchant.PickupZoneID,
	}
}

type merchantOrderItem struct {
	OrderID         string         `json:"orderId"`
	ExternalOrderID string         `json:"externalOrderId"`
	TrackingNumber  string         `json:"trackingNumber"`
	Status          string         `json:"status"`
	RecipientName   string         `json:"recipientName"`
	DropoffCity     string         `json:"dropoffCity"`
	Total           orders.Decimal `json:"total"`
	Currency        string         `json:"currency"`
	CreatedAt       time.Time      `json:"createdAt"`
	UpdatedAt       time.Time      `json:"updatedAt"`
}

type merchantOrderDetail struct {
	merchantOrderItem
	WeightKG     orders.Decimal         `json:"weightKg"`
	SizeCategory string                 `json:"sizeCategory"`
	Description  *string                `json:"description"`
	Fee          orders.Fee             `json:"fee"`
	Timeline     []orders.TimelineEvent `json:"timeline"`
}

type merchantOrderCreateResponse struct {
	OrderID         string     `json:"orderId"`
	ExternalOrderID string     `json:"externalOrderId"`
	TrackingNumber  string     `json:"trackingNumber"`
	Status          string     `json:"status"`
	Fee             orders.Fee `json:"fee"`
	CreatedAt       time.Time  `json:"createdAt"`
}

func orderItem(order orders.Order) merchantOrderItem {
	return merchantOrderItem{
		OrderID: order.ID, ExternalOrderID: order.ExternalOrderID, TrackingNumber: order.TrackingNumber,
		Status: order.Status, RecipientName: order.Recipient.Name, DropoffCity: order.Recipient.City,
		Total: order.Fee.Total, Currency: order.Fee.Currency, CreatedAt: order.CreatedAt, UpdatedAt: order.UpdatedAt,
	}
}

func orderCreateResponse(order orders.Order) merchantOrderCreateResponse {
	return merchantOrderCreateResponse{
		OrderID: order.ID, ExternalOrderID: order.ExternalOrderID, TrackingNumber: order.TrackingNumber,
		Status: order.Status, Fee: order.Fee, CreatedAt: order.CreatedAt,
	}
}

func orderDetailResponse(detail orders.Detail) merchantOrderDetail {
	return merchantOrderDetail{
		merchantOrderItem: orderItem(detail.Order),
		WeightKG:          detail.WeightKG,
		SizeCategory:      detail.SizeCategory,
		Description:       detail.Description,
		Fee:               detail.Fee,
		Timeline:          detail.Timeline,
	}
}

func parseOrderList(c *gin.Context, merchantID string) (orders.ListFilter, []validation.Issue) {
	issues := validation.New()
	filter := orders.ListFilter{MerchantID: merchantID, Page: 1, PageSize: 20, Sort: "createdAt", Desc: true}

	if raw, ok := c.GetQuery("page"); ok {
		page, err := strconv.Atoi(raw)
		if err != nil || page < 1 {
			issues.Add("page", "must be a positive integer")
		} else {
			filter.Page = page
		}
	}
	if raw, ok := c.GetQuery("pageSize"); ok {
		size, err := strconv.Atoi(raw)
		if err != nil || size < 1 || size > 100 {
			issues.Add("pageSize", "must be between 1 and 100")
		} else {
			filter.PageSize = size
		}
	}

	status := strings.ToUpper(strings.TrimSpace(c.Query("status")))
	if status != "" {
		if !validOrderStatus(status) {
			issues.Add("status", "must be CREATED, PICKED_UP, IN_TRANSIT, OUT_FOR_DELIVERY, DELIVERED or FAILED")
		} else {
			filter.Status = status
		}
	}

	external := strings.TrimSpace(c.Query("externalOrderId"))
	if external != "" {
		if strings.ContainsRune(external, '\x00') || validation.RuneLength(external) > 100 {
			issues.Add("externalOrderId", "must be at most 100 characters")
		} else {
			filter.ExternalOrderID = external
		}
	}

	var from, to time.Time
	if raw, ok := c.GetQuery("from"); ok && raw != "" {
		date, err := time.Parse("2006-01-02", raw)
		if err != nil {
			issues.Add("from", "must be an ISO date")
		} else {
			from = date.UTC()
			filter.From = &from
		}
	}
	if raw, ok := c.GetQuery("to"); ok && raw != "" {
		date, err := time.Parse("2006-01-02", raw)
		if err != nil {
			issues.Add("to", "must be an ISO date")
		} else {
			to = date.UTC().Add(24 * time.Hour)
			filter.To = &to
		}
	}
	if !from.IsZero() && !to.IsZero() && !to.After(from) {
		issues.Add("to", "must be on or after the from date")
	}

	if raw, ok := c.GetQuery("sort"); ok && raw != "" {
		field, direction, found := strings.Cut(raw, ":")
		if found {
			switch direction {
			case "asc":
				filter.Desc = false
			case "desc":
				filter.Desc = true
			default:
				issues.Add("sort", "direction must be asc or desc")
			}
		}
		switch field {
		case "createdAt":
			filter.Sort = "createdAt"
		case "updatedAt":
			filter.Sort = "updatedAt"
		case "externalOrderId":
			filter.Sort = "externalOrderId"
		default:
			issues.Add("sort", "must be createdAt, updatedAt or externalOrderId")
		}
	}
	return filter, issues.Issues()
}

func validOrderStatus(status string) bool {
	switch status {
	case "CREATED", "PICKED_UP", "IN_TRANSIT", "OUT_FOR_DELIVERY", "DELIVERED", "FAILED":
		return true
	default:
		return false
	}
}
