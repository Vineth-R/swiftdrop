package servicehttp

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Vineth-R/swiftdrop/apps/services/identity/internal/service"
	"github.com/Vineth-R/swiftdrop/packages/go-common/httpx"
	"github.com/Vineth-R/swiftdrop/packages/go-common/requestid"
)

const (
	// googleLandingPath is where a successful Google login goes when the caller
	// did not ask for a specific page (API v1.1 section 2.6).
	googleLandingPath = "/dashboard"

	// googleFailurePath is where a failed Google login sends the browser.
	googleFailurePath = "/login?error=oauth_failed"
)

// GoogleService is the Google sign-in use cases the handlers call.
// *service.GoogleAuth satisfies it.
type GoogleService interface {
	StartGoogleLogin(ctx context.Context, redirectTo string) (service.GoogleLoginStart, error)
	CompleteGoogleLogin(ctx context.Context, input service.GoogleCallbackInput) (service.GoogleLoginResult, error)
}

// GoogleRoutes carries what the Google endpoints need.
type GoogleRoutes struct {
	Service GoogleService
	Cookies CookieConfig
}

// WithGoogle mounts the Google sign-in endpoints (API v1.1 sections 2.5 and
// 2.6). Leave it out to run without Google sign-in.
func WithGoogle(routes GoogleRoutes) Option {
	return func(options *routerOptions) {
		options.google = &routes
	}
}

type googleHandlers struct {
	service GoogleService
	cookies CookieConfig
	logger  *slog.Logger
	// auth supplies the shared error mapping for validation errors, disabled
	// accounts and unexpected failures.
	auth *authHandlers
}

func registerGoogleRoutes(router *gin.Engine, routes GoogleRoutes, logger *slog.Logger) {
	handlers := &googleHandlers{
		service: routes.Service,
		cookies: routes.Cookies,
		logger:  logger,
		auth:    &authHandlers{cookies: routes.Cookies, logger: logger},
	}
	limiter := newRateLimiter(authAttemptsPerMinute, time.Minute, nil)

	group := router.Group("/auth/google", noStore())
	group.GET("/login", limitByClient(limiter), handlers.login)
	group.GET("/callback", limitByClient(limiter), handlers.callback)
}

// login handles GET /auth/google/login (API v1.1 section 2.5): it sends the
// browser to Google's consent page.
func (h *googleHandlers) login(c *gin.Context) {
	start, err := h.service.StartGoogleLogin(c.Request.Context(), c.Query("redirectTo"))
	if err != nil {
		h.fail(c, err)
		return
	}
	h.cookies.SetOAuthBinding(c, start.BindingCookieName, start.BindingValue, start.BindingMaxAge)
	c.Redirect(http.StatusFound, start.AuthURL)
}

// callback handles GET /auth/google/callback (API v1.1 section 2.6). A browser
// is redirected on success and on failure; a caller that asks for JSON gets the
// documented JSON error instead.
func (h *googleHandlers) callback(c *gin.Context) {
	code := c.Query("code")
	if c.Query("error") != "" {
		// Google reports a refusal (for example the user pressed Cancel) with
		// error=... and no code. Passing no code still burns the state.
		code = ""
	}

	state := c.Query("state")
	cookieName, hasCookieName := service.OAuthBindingCookieName(state)
	binding := ""
	if hasCookieName {
		if value, err := c.Cookie(cookieName); err == nil {
			binding = value
		}
	}

	result, err := h.service.CompleteGoogleLogin(c.Request.Context(), service.GoogleCallbackInput{
		Code:           code,
		State:          state,
		BrowserBinding: binding,
		UserAgent:      c.Request.UserAgent(),
		IP:             clientAddress(c),
	})
	if hasCookieName {
		// The binding is single-use; remove it whether the callback succeeded
		// or failed.
		h.cookies.ClearOAuthBinding(c, cookieName)
	}
	if err != nil {
		h.callbackFailed(c, err)
		return
	}

	h.cookies.SetSession(c, result.Session)

	// The path was validated when the login started; check again so a bad row
	// could never turn this into an open redirect.
	target := result.RedirectTo
	if !service.ValidRedirectPath(target) {
		target = googleLandingPath
	}
	c.Redirect(http.StatusFound, target)
}

func (h *googleHandlers) callbackFailed(c *gin.Context, err error) {
	if prefersJSON(c) {
		h.fail(c, err)
		return
	}

	// Never log the code, the state or any token; the error text carries none.
	level := slog.LevelWarn
	if !isExpectedGoogleFailure(err) {
		level = slog.LevelError
	}
	h.logger.Log(c.Request.Context(), level, "google sign-in failed",
		"request_id", requestid.FromGin(c),
		"error", err, "error_type", fmt.Sprintf("%T", err),
	)
	c.Redirect(http.StatusFound, googleFailurePath)
}

// fail maps a Google service error to its API v1.1 JSON response.
func (h *googleHandlers) fail(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrOAuthStateInvalid):
		httpx.Error(c, http.StatusBadRequest, "OAUTH_STATE_INVALID", "the sign-in request is invalid or has expired")
	case errors.Is(err, service.ErrOAuthProvider):
		h.logger.WarnContext(c.Request.Context(), "google sign-in provider error",
			"request_id", requestid.FromGin(c),
			"error", err, "error_type", fmt.Sprintf("%T", err),
		)
		httpx.Error(c, http.StatusBadGateway, "OAUTH_PROVIDER_ERROR", "google sign-in failed")
	case errors.Is(err, service.ErrGoogleEmailNotVerified):
		httpx.Error(c, http.StatusForbidden, "FORBIDDEN", "the google account email is not verified")
	default:
		h.auth.fail(c, err)
	}
}

// isExpectedGoogleFailure separates failures that are normal (an expired
// state, a cancelled consent, a disabled account) from real faults.
func isExpectedGoogleFailure(err error) bool {
	return errors.Is(err, service.ErrOAuthStateInvalid) ||
		errors.Is(err, service.ErrOAuthProvider) ||
		errors.Is(err, service.ErrGoogleEmailNotVerified) ||
		errors.Is(err, service.ErrAccountDisabled)
}

// prefersJSON reports whether the caller is an API client rather than a
// browser navigation. Browsers always list text/html in Accept.
func prefersJSON(c *gin.Context) bool {
	accept := c.GetHeader("Accept")
	return strings.Contains(accept, "application/json") && !strings.Contains(accept, "text/html")
}
