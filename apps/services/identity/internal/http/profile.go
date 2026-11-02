package servicehttp

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Vineth-R/swiftdrop/apps/services/identity/internal/service"
	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
	"github.com/Vineth-R/swiftdrop/packages/go-common/httpx"
	"github.com/Vineth-R/swiftdrop/packages/go-common/requestid"
)

// ProfileService is the own-account use case the handler calls.
// *service.Service satisfies it.
type ProfileService interface {
	Profile(ctx context.Context, userID string) (service.Profile, error)
}

// ProfileRoutes carries what the own-account endpoints need.
type ProfileRoutes struct {
	Service  ProfileService
	Verifier auth.Verifier
	Cookies  CookieConfig
}

// WithProfile mounts GET /users/me (API v1.1 section 2.7, any signed-in user).
// PATCH /users/me and PUT /users/me/password (sections 2.8 and 2.9) are added to
// the same group when they are built.
func WithProfile(routes ProfileRoutes) Option {
	return func(options *routerOptions) {
		options.profile = &routes
	}
}

type profileHandlers struct {
	service ProfileService
	logger  *slog.Logger
}

// profileBody is the user as the user sees them (API v1.1 section 2.7).
type profileBody struct {
	ID           string    `json:"id"`
	Email        string    `json:"email"`
	FullName     string    `json:"fullName"`
	Phone        string    `json:"phone"`
	Role         auth.Role `json:"role"`
	AuthProvider string    `json:"authProvider"`
	CreatedAt    string    `json:"createdAt"`
}

func registerProfileRoutes(router *gin.Engine, routes ProfileRoutes, logger *slog.Logger) {
	handlers := &profileHandlers{service: routes.Service, logger: logger}
	limiter := newRateLimiter(userRequestsPerMinute, time.Minute, nil)

	// The identity comes from the signed token and never from the request, so a
	// user can only ever read their own account.
	me := router.Group("/users/me", noStore(), auth.Authenticate(routes.Verifier, routes.Cookies.AccessName), limitByUser(limiter))
	me.GET("", handlers.get)
}

// get handles GET /users/me (API v1.1 section 2.7).
func (h *profileHandlers) get(c *gin.Context) {
	claims, ok := auth.ClaimsFrom(c)
	if !ok {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}

	profile, err := h.service.Profile(c.Request.Context(), claims.UserID)
	switch {
	case errors.Is(err, service.ErrUserNotFound):
		// The token is valid but the account is gone: the session is over.
		httpx.Error(c, http.StatusUnauthorized, "UNAUTHENTICATED", "authentication required")
	case errors.Is(err, service.ErrAccountDisabled):
		httpx.Error(c, http.StatusForbidden, "ACCOUNT_DISABLED", "this account has been disabled")
	case err != nil:
		h.logger.ErrorContext(c.Request.Context(), "could not read the signed-in user's profile",
			"request_id", requestid.FromGin(c), "path", c.FullPath(), "error", err.Error())
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
	default:
		httpx.Success(c, http.StatusOK, profileBody{
			ID:           profile.ID,
			Email:        profile.Email,
			FullName:     profile.FullName,
			Phone:        profile.Phone,
			Role:         profile.Role,
			AuthProvider: profile.AuthProvider,
			CreatedAt:    profile.CreatedAt.UTC().Format(time.RFC3339),
		})
	}
}
