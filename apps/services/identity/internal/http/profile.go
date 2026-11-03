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

// ProfileService is the own-account use cases the handlers call.
// *service.Service satisfies it.
type ProfileService interface {
	Profile(ctx context.Context, userID string) (service.Profile, error)
	UpdateProfile(ctx context.Context, userID string, input service.UpdateProfileInput) (service.Profile, error)
	ChangePassword(ctx context.Context, userID string, input service.ChangePasswordInput) error
}

// ProfileRoutes carries what the own-account endpoints need.
type ProfileRoutes struct {
	Service  ProfileService
	Verifier auth.Verifier
	Cookies  CookieConfig
}

// WithProfile mounts the own-account endpoints, for any signed-in user: GET and
// PATCH /users/me and PUT /users/me/password (API v1.1 sections 2.7 to 2.9).
func WithProfile(routes ProfileRoutes) Option {
	return func(options *routerOptions) {
		options.profile = &routes
	}
}

// passwordChangesPerMinute limits PUT /users/me/password per signed-in user.
// The endpoint checks a password, so it gets a far lower limit than the rest of
// the group: a stolen session cannot be used to guess the current password.
const passwordChangesPerMinute = 5

type profileHandlers struct {
	service ProfileService
	cookies CookieConfig
	logger  *slog.Logger
}

// updateProfileRequest is PATCH /users/me. A field left out stays unchanged.
// Any other field in the body is ignored, so the email and role cannot be set
// here.
type updateProfileRequest struct {
	FullName *string `json:"fullName"`
	Phone    *string `json:"phone"`
}

type changePasswordRequest struct {
	CurrentPassword string `json:"currentPassword"`
	NewPassword     string `json:"newPassword"`
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
	handlers := &profileHandlers{service: routes.Service, cookies: routes.Cookies, logger: logger}
	limiter := newRateLimiter(userRequestsPerMinute, time.Minute, nil)
	passwordLimiter := newRateLimiter(passwordChangesPerMinute, time.Minute, nil)

	// The identity comes from the signed token and never from the request, so a
	// user can only ever read their own account.
	me := router.Group("/users/me", noStore(), auth.Authenticate(routes.Verifier, routes.Cookies.AccessName), limitByUser(limiter))
	me.GET("", handlers.get)
	me.PATCH("", handlers.update)
	me.PUT("/password", limitByUser(passwordLimiter), handlers.changePassword)
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
		httpx.Success(c, http.StatusOK, profileResponse(profile))
	}
}

// update handles PATCH /users/me (API v1.1 section 2.8). It answers with the
// updated account, in the shape of section 2.7.
func (h *profileHandlers) update(c *gin.Context) {
	claims, ok := auth.ClaimsFrom(c)
	if !ok {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}
	var request updateProfileRequest
	if !bindJSON(c, &request) {
		return
	}

	profile, err := h.service.UpdateProfile(c.Request.Context(), claims.UserID, service.UpdateProfileInput{
		FullName: request.FullName,
		Phone:    request.Phone,
	})
	if err != nil {
		h.fail(c, err)
		return
	}
	httpx.Success(c, http.StatusOK, profileResponse(profile))
}

// changePassword handles PUT /users/me/password (API v1.1 section 2.9). Every
// session of the user ends, this browser's included, so the cookies are cleared
// and the person signs in again with the new password.
func (h *profileHandlers) changePassword(c *gin.Context) {
	claims, ok := auth.ClaimsFrom(c)
	if !ok {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}
	var request changePasswordRequest
	if !bindJSON(c, &request) {
		return
	}

	err := h.service.ChangePassword(c.Request.Context(), claims.UserID, service.ChangePasswordInput{
		CurrentPassword: request.CurrentPassword,
		NewPassword:     request.NewPassword,
	})
	if err != nil {
		h.fail(c, err)
		return
	}
	h.cookies.Clear(c)
	c.Status(http.StatusNoContent)
}

// fail maps an error of the write endpoints to its API v1.1 response.
// Unexpected errors are logged once here, without request data, and answered
// with a generic 500.
func (h *profileHandlers) fail(c *gin.Context, err error) {
	var validation *service.ValidationError
	switch {
	case errors.As(err, &validation):
		details := make([]httpx.ErrorItem, 0, len(validation.Issues))
		for _, issue := range validation.Issues {
			details = append(details, httpx.ErrorItem{Field: issue.Field, Issue: issue.Issue})
		}
		httpx.ErrorWithDetails(c, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "One or more fields are invalid.", details)
	case errors.Is(err, service.ErrUserNotFound):
		// The token is valid but the account is gone: the session is over.
		httpx.Error(c, http.StatusUnauthorized, "UNAUTHENTICATED", "authentication required")
	case errors.Is(err, service.ErrAccountDisabled):
		httpx.Error(c, http.StatusForbidden, "ACCOUNT_DISABLED", "this account has been disabled")
	case errors.Is(err, service.ErrInvalidCredentials):
		httpx.Error(c, http.StatusUnauthorized, "INVALID_CREDENTIALS", "the current password is incorrect")
	case errors.Is(err, service.ErrPasswordNotSet):
		httpx.Error(c, http.StatusConflict, "PASSWORD_NOT_SET", "this account signs in with Google and has no password to change")
	default:
		h.logger.ErrorContext(c.Request.Context(), "own-account request failed",
			"request_id", requestid.FromGin(c), "path", c.FullPath(), "error", err.Error())
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
	}
}

func profileResponse(profile service.Profile) profileBody {
	return profileBody{
		ID:           profile.ID,
		Email:        profile.Email,
		FullName:     profile.FullName,
		Phone:        profile.Phone,
		Role:         profile.Role,
		AuthProvider: profile.AuthProvider,
		CreatedAt:    profile.CreatedAt.UTC().Format(time.RFC3339),
	}
}
