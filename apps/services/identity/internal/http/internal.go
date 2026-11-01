package servicehttp

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/Vineth-R/swiftdrop/apps/services/identity/internal/service"
	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
	"github.com/Vineth-R/swiftdrop/packages/go-common/httpx"
	"github.com/Vineth-R/swiftdrop/packages/go-common/requestid"
)

// internalTokenHeader carries the shared secret between services (API v1.1
// section 1.2).
const internalTokenHeader = "X-Internal-Token"

// InternalService is the service-to-service lookups the handlers call.
// *service.InternalAPI satisfies it.
type InternalService interface {
	CourierEligibility(ctx context.Context, courierID string) (service.CourierEligibility, error)
	UserContact(ctx context.Context, userID string) (service.UserContact, error)
}

// InternalRoutes carries what the internal endpoints need. An empty Token
// closes the endpoints: every request is refused.
type InternalRoutes struct {
	Service InternalService
	Token   string
}

// WithInternal mounts the internal endpoints (API v1.1 section 13). Nginx never
// routes /internal, so only services on the Compose network reach them.
func WithInternal(routes InternalRoutes) Option {
	return func(options *routerOptions) {
		options.internal = &routes
	}
}

type internalHandlers struct {
	service InternalService
	logger  *slog.Logger
}

type courierEligibilityResponse struct {
	IsActive    bool      `json:"isActive"`
	IsAvailable bool      `json:"isAvailable"`
	Role        auth.Role `json:"role"`
}

type userContactResponse struct {
	FullName string `json:"fullName"`
	Email    string `json:"email"`
	Phone    string `json:"phone"`
}

func registerInternalRoutes(router *gin.Engine, routes InternalRoutes, logger *slog.Logger) {
	handlers := &internalHandlers{service: routes.Service, logger: logger}

	group := router.Group("/internal", noStore(), requireInternalToken(routes.Token), requireRequestID())
	group.GET("/couriers/:courierId/eligibility", handlers.courierEligibility)
	group.GET("/users/:userId/contact", handlers.userContact)
}

// requireInternalToken refuses any request whose X-Internal-Token does not
// match token. It fails closed: with an empty token nothing matches, even a
// request that sends no header, so a missing configuration can never open the
// endpoints. The comparison takes the same time whatever the input.
func requireInternalToken(token string) gin.HandlerFunc {
	expected := sha256.Sum256([]byte(token))
	configured := token != ""

	return func(c *gin.Context) {
		supplied := sha256.Sum256([]byte(c.GetHeader(internalTokenHeader)))
		// Comparing fixed-length digests hides the length of the real token.
		matches := subtle.ConstantTimeCompare(expected[:], supplied[:]) == 1
		if !configured || !matches || c.GetHeader(internalTokenHeader) == "" {
			httpx.Error(c, http.StatusUnauthorized, "UNAUTHENTICATED", "authentication required")
			return
		}
		c.Next()
	}
}

// requireRequestID demands a well-formed X-Request-Id from the caller, as the
// internal contract says every call carries one. Without it a failure cannot
// be traced across services.
func requireRequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !requestid.Valid(c.GetHeader(requestid.Header)) {
			httpx.Error(c, http.StatusBadRequest, "BAD_REQUEST", "a valid X-Request-Id header is required")
			return
		}
		c.Next()
	}
}

// courierEligibility handles GET /internal/couriers/{courierId}/eligibility.
func (h *internalHandlers) courierEligibility(c *gin.Context) {
	eligibility, err := h.service.CourierEligibility(c.Request.Context(), c.Param("courierId"))
	if err != nil {
		h.fail(c, err)
		return
	}
	httpx.Success(c, http.StatusOK, courierEligibilityResponse{
		IsActive:    eligibility.IsActive,
		IsAvailable: eligibility.IsAvailable,
		Role:        eligibility.Role,
	})
}

// userContact handles GET /internal/users/{userId}/contact.
func (h *internalHandlers) userContact(c *gin.Context) {
	contact, err := h.service.UserContact(c.Request.Context(), c.Param("userId"))
	if err != nil {
		h.fail(c, err)
		return
	}
	httpx.Success(c, http.StatusOK, userContactResponse{
		FullName: contact.FullName,
		Email:    contact.Email,
		Phone:    contact.Phone,
	})
}

func (h *internalHandlers) fail(c *gin.Context, err error) {
	if errors.Is(err, service.ErrUserNotFound) {
		httpx.Error(c, http.StatusNotFound, "NOT_FOUND", "user not found")
		return
	}
	h.logger.ErrorContext(c.Request.Context(), "internal request failed",
		"request_id", requestid.FromGin(c),
		"path", c.FullPath(),
		"error", err.Error(),
	)
	httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
}
