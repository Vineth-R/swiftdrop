package servicehttp

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/Vineth-R/swiftdrop/apps/services/identity/internal/repository"
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
	CreateMerchantOwner(ctx context.Context, input service.MerchantOwnerInput) (service.MerchantOwnerResult, error)
	ApplyMerchantLifecycle(ctx context.Context, input service.LifecycleInput) (service.LifecycleResult, error)
	SessionState(ctx context.Context, userID string) (service.SessionState, error)
	DeleteMerchantOwner(ctx context.Context, userID string) error
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
	group.GET("/users/:userId/session-state", handlers.sessionState)
	group.POST("/merchant-owners", handlers.createMerchantOwner)
	group.POST("/merchant-owners/:userId/lifecycle", handlers.lifecycle)
	group.DELETE("/merchant-owners/:userId", handlers.deleteMerchantOwner)
}

type merchantOwnerRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	FullName string `json:"fullName"`
	Phone    string `json:"phone"`
}

type lifecycleRequest struct {
	TransitionID string `json:"transitionId"`
	Sequence     int64  `json:"sequence"`
	Operation    string `json:"operation"`
}

func (h *internalHandlers) sessionState(c *gin.Context) {
	state, err := h.service.SessionState(c.Request.Context(), c.Param("userId"))
	if err != nil {
		h.fail(c, err)
		return
	}
	httpx.Success(c, http.StatusOK, gin.H{
		"role": state.Role, "isActive": state.IsActive, "sessionVersion": state.SessionVersion,
	})
}

func (h *internalHandlers) createMerchantOwner(c *gin.Context) {
	var request merchantOwnerRequest
	if !bindJSON(c, &request) {
		return
	}
	created, err := h.service.CreateMerchantOwner(c.Request.Context(), service.MerchantOwnerInput{
		Email: request.Email, Password: request.Password, FullName: request.FullName, Phone: request.Phone,
	})
	if err != nil {
		h.fail(c, err)
		return
	}
	status := http.StatusOK
	if created.Created {
		status = http.StatusCreated
	}
	httpx.Success(c, status, gin.H{"userId": created.UserID, "created": created.Created})
}

func (h *internalHandlers) lifecycle(c *gin.Context) {
	var request lifecycleRequest
	if !bindJSON(c, &request) {
		return
	}
	applied, err := h.service.ApplyMerchantLifecycle(c.Request.Context(), service.LifecycleInput{
		UserID: c.Param("userId"), TransitionID: request.TransitionID,
		Sequence: request.Sequence, Operation: request.Operation,
	})
	if err != nil {
		h.fail(c, err)
		return
	}
	httpx.Success(c, http.StatusOK, gin.H{
		"userId": applied.UserID, "transitionId": applied.TransitionID,
		"sequence": applied.Sequence, "operation": applied.Operation,
		"alreadyApplied": applied.AlreadyApplied, "sessionVersion": applied.SessionVersion,
		"isActive": applied.IsActive, "revokedSessions": applied.RevokedSessions,
	})
}

func (h *internalHandlers) deleteMerchantOwner(c *gin.Context) {
	if err := h.service.DeleteMerchantOwner(c.Request.Context(), c.Param("userId")); err != nil {
		h.fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
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
	var validationErr *service.ValidationError
	switch {
	case errors.Is(err, service.ErrUserNotFound):
		httpx.Error(c, http.StatusNotFound, "NOT_FOUND", "user not found")
		return
	case errors.As(err, &validationErr):
		httpx.ErrorWithDetails(c, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "One or more fields are invalid.", httpx.ErrorItems(validationErr.Issues))
		return
	case errors.Is(err, repository.ErrEmailTaken):
		httpx.Error(c, http.StatusConflict, "EMAIL_ALREADY_EXISTS", "an account with this email already exists")
		return
	case errors.Is(err, repository.ErrTransitionConflict):
		httpx.Error(c, http.StatusConflict, "TRANSITION_ID_CONFLICT", "transition id conflicts with an existing transition")
		return
	case errors.Is(err, repository.ErrTransitionStale):
		httpx.Error(c, http.StatusConflict, "STALE_TRANSITION", "transition sequence is stale")
		return
	}
	h.logger.ErrorContext(c.Request.Context(), "internal request failed",
		"request_id", requestid.FromGin(c),
		"path", c.FullPath(),
		"error", err.Error(),
	)
	httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
}
