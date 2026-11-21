package servicehttp

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Vineth-R/swiftdrop/apps/services/identity/internal/service"
	"github.com/Vineth-R/swiftdrop/packages/go-common/httpx"
)

const (
	// resetAttemptsPerMinute is the limit for /auth/forgot-password and
	// /auth/reset-password per client address, and for forgot-password per
	// submitted email (API v1.1 section 1.7: recommended 5 per minute).
	resetAttemptsPerMinute = 5

	forgotPasswordMessage = "If an account exists for this email, a password reset link will be sent."
	resetPasswordMessage  = "Password has been reset successfully."
)

// PasswordResetService is the reset use cases the handlers call.
// *service.PasswordReset satisfies it.
type PasswordResetService interface {
	ForgotPassword(ctx context.Context, input service.ForgotPasswordInput) error
	ResetPassword(ctx context.Context, input service.ResetPasswordInput) error
}

// PasswordResetRoutes carries what the reset endpoints need. Cookies are
// cleared after a successful reset so the browser does not keep dead sessions.
type PasswordResetRoutes struct {
	Service PasswordResetService
	Cookies CookieConfig
}

// WithPasswordReset mounts POST /auth/forgot-password and POST
// /auth/reset-password (API v1.1 sections 2.16 and 2.17).
func WithPasswordReset(routes PasswordResetRoutes) Option {
	return func(options *routerOptions) {
		options.reset = &routes
	}
}

type resetHandlers struct {
	service PasswordResetService
	cookies CookieConfig
	// emailLimiter caps forgot-password requests per submitted email, so one
	// inbox cannot be flooded from many addresses.
	emailLimiter *rateLimiter
	// auth supplies the shared mapping for validation errors and unexpected
	// failures.
	auth *authHandlers
}

type forgotPasswordRequest struct {
	Email string `json:"email"`
}

type resetPasswordRequest struct {
	Token       string `json:"token"`
	NewPassword string `json:"newPassword"`
}

type messageResponse struct {
	Message string `json:"message"`
}

func registerPasswordResetRoutes(router *gin.Engine, routes PasswordResetRoutes, logger *slog.Logger) {
	handlers := &resetHandlers{
		service:      routes.Service,
		cookies:      routes.Cookies,
		emailLimiter: newRateLimiter(resetAttemptsPerMinute, time.Minute, nil),
		auth:         &authHandlers{cookies: routes.Cookies, logger: logger},
	}
	forgotLimiter := newRateLimiter(resetAttemptsPerMinute, time.Minute, nil)
	resetLimiter := newRateLimiter(resetAttemptsPerMinute, time.Minute, nil)

	group := router.Group("/auth", noStore())
	group.POST("/forgot-password", limitByClient(forgotLimiter), handlers.forgotPassword)
	group.POST("/reset-password", limitByClient(resetLimiter), handlers.resetPassword)
}

// forgotPassword handles POST /auth/forgot-password (API v1.1 section 2.16).
// Every well-formed request gets the same 202 reply.
func (h *resetHandlers) forgotPassword(c *gin.Context) {
	var request forgotPasswordRequest
	if !bindJSON(c, &request) {
		return
	}

	// The limiter key is the normalised email, so "A@x.com" and "a@x.com " share
	// a budget. A blank email is left to validation below.
	if email := strings.ToLower(strings.TrimSpace(request.Email)); email != "" {
		if allowed, _, resetIn := h.emailLimiter.allow(email); !allowed {
			rejectRateLimited(c, resetIn)
			return
		}
	}

	if err := h.service.ForgotPassword(c.Request.Context(), service.ForgotPasswordInput{
		Email: request.Email,
		IP:    clientAddress(c),
	}); err != nil {
		h.fail(c, err)
		return
	}
	httpx.Success(c, http.StatusAccepted, messageResponse{Message: forgotPasswordMessage})
}

// resetPassword handles POST /auth/reset-password (API v1.1 section 2.17).
func (h *resetHandlers) resetPassword(c *gin.Context) {
	var request resetPasswordRequest
	if !bindJSON(c, &request) {
		return
	}

	if err := h.service.ResetPassword(c.Request.Context(), service.ResetPasswordInput{
		Token:       request.Token,
		NewPassword: request.NewPassword,
	}); err != nil {
		h.fail(c, err)
		return
	}

	// Every session of this user was just revoked; drop the cookies the browser
	// may still hold.
	h.cookies.Clear(c)
	httpx.Success(c, http.StatusOK, messageResponse{Message: resetPasswordMessage})
}

// rejectRateLimited answers 429 RATE_LIMITED in the same shape as
// limitByClient, for limits keyed on something other than the client address.
func rejectRateLimited(c *gin.Context, resetIn time.Duration) {
	c.Header("X-RateLimit-Remaining", "0")
	c.Header("Retry-After", strconv.Itoa(int(resetIn/time.Second)+1))
	httpx.Error(c, http.StatusTooManyRequests, "RATE_LIMITED", "too many requests, try again later")
}

func (h *resetHandlers) fail(c *gin.Context, err error) {
	if errors.Is(err, service.ErrResetTokenInvalid) {
		// Unknown, expired and used tokens share one message (API v1.1 section 2.17).
		httpx.Error(c, http.StatusBadRequest, "INVALID_OR_EXPIRED_RESET_TOKEN", "the reset link is invalid or has expired")
		return
	}
	h.auth.fail(c, err)
}
