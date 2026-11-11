package servicehttp

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Vineth-R/swiftdrop/apps/services/identity/internal/service"
	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
	"github.com/Vineth-R/swiftdrop/packages/go-common/httpx"
	"github.com/Vineth-R/swiftdrop/packages/go-common/requestid"
	"github.com/Vineth-R/swiftdrop/packages/go-common/validation"
)

const (
	// maxBodyBytes caps JSON request bodies on the auth endpoints.
	maxBodyBytes = 16 << 10

	tokenModeHeader = "X-Token-Mode"
	tokenModeBody   = "body"
)

// AuthService is the identity use cases the auth endpoints call.
// *service.Service satisfies it.
type AuthService interface {
	Register(ctx context.Context, input service.RegisterInput) (service.Account, error)
	Login(ctx context.Context, input service.LoginInput) (service.Session, error)
	Refresh(ctx context.Context, input service.RefreshInput) (service.Session, error)
	Logout(ctx context.Context, userID, refreshToken string) error
	VerifyEmail(ctx context.Context, token string) error
	ResendVerification(ctx context.Context, email string, ip netip.Addr) error
}

// AuthRoutes carries what the auth endpoints need.
type AuthRoutes struct {
	Service  AuthService
	Verifier auth.Verifier
	Cookies  CookieConfig
}

type authHandlers struct {
	service AuthService
	cookies CookieConfig
	logger  *slog.Logger
}

type registerRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	FullName string `json:"fullName"`
	Phone    string `json:"phone"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type refreshRequest struct {
	RefreshToken string `json:"refreshToken"`
}

type accountResponse struct {
	ID            string    `json:"id"`
	Email         string    `json:"email"`
	FullName      string    `json:"fullName"`
	Phone         string    `json:"phone,omitempty"`
	Role          auth.Role `json:"role"`
	EmailVerified bool      `json:"emailVerified"`
	CreatedAt     string    `json:"createdAt,omitempty"`
}

type sessionResponse struct {
	User         *accountResponse `json:"user,omitempty"`
	ExpiresIn    int              `json:"expiresIn"`
	AccessToken  string           `json:"accessToken,omitempty"`
	RefreshToken string           `json:"refreshToken,omitempty"`
}

func registerAuthRoutes(router *gin.Engine, routes AuthRoutes, logger *slog.Logger) {
	handlers := &authHandlers{service: routes.Service, cookies: routes.Cookies, logger: logger}
	limiter := newRateLimiter(authAttemptsPerMinute, time.Minute, nil)

	group := router.Group("/auth", noStore())
	group.POST("/register", limitByClient(limiter), handlers.register)
	group.POST("/login", limitByClient(limiter), handlers.login)
	group.POST("/refresh", handlers.refresh)
	group.POST("/logout", auth.Authenticate(routes.Verifier, routes.Cookies.AccessName), handlers.logout)

	verifyLimiter := newRateLimiter(verificationAttemptsPerMinute, time.Minute, nil)
	resendLimiter := newRateLimiter(verificationAttemptsPerMinute, time.Minute, nil)
	resendEmailLimiter := newRateLimiter(verificationAttemptsPerMinute, time.Minute, nil)
	group.POST("/verify-email", limitByClient(verifyLimiter), handlers.verifyEmail)
	group.POST("/resend-verification", limitByClient(resendLimiter), handlers.resendVerification(resendEmailLimiter))
}

const (
	verificationAttemptsPerMinute = 5
	verifyEmailMessage            = "Your email has been verified. You can now sign in to SwiftDrop."
	resendVerificationMessage     = "If an account needs verification, a new verification email will be sent."
)

type verifyEmailRequest struct {
	Token string `json:"token"`
}

type resendVerificationRequest struct {
	Email string `json:"email"`
}

func (h *authHandlers) verifyEmail(c *gin.Context) {
	var request verifyEmailRequest
	if !bindJSON(c, &request) {
		return
	}
	if err := h.service.VerifyEmail(c.Request.Context(), request.Token); err != nil {
		h.fail(c, err)
		return
	}
	httpx.Success(c, http.StatusOK, messageResponse{Message: verifyEmailMessage})
}

func (h *authHandlers) resendVerification(emailLimiter *rateLimiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		var request resendVerificationRequest
		if !bindJSON(c, &request) {
			return
		}
		email := strings.ToLower(strings.TrimSpace(request.Email))
		if email != "" {
			if allowed, _, resetIn := emailLimiter.allow(email); !allowed {
				rejectRateLimited(c, resetIn)
				return
			}
		}
		if err := h.service.ResendVerification(c.Request.Context(), request.Email, clientAddress(c)); err != nil {
			h.fail(c, err)
			return
		}
		httpx.Success(c, http.StatusAccepted, messageResponse{Message: resendVerificationMessage})
	}
}

// noStore keeps tokens out of browser and proxy caches.
func noStore() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		c.Next()
	}
}

// register handles POST /auth/register (API v1.1 section 2.1).
func (h *authHandlers) register(c *gin.Context) {
	var request registerRequest
	if !bindJSON(c, &request) {
		return
	}

	account, err := h.service.Register(c.Request.Context(), service.RegisterInput{
		Email:    request.Email,
		Password: request.Password,
		FullName: request.FullName,
		Phone:    request.Phone,
	})
	if err != nil {
		h.fail(c, err)
		return
	}
	httpx.Success(c, http.StatusCreated, accountBody(account))
}

// login handles POST /auth/login (API v1.1 section 2.2).
func (h *authHandlers) login(c *gin.Context) {
	var request loginRequest
	if !bindJSON(c, &request) {
		return
	}

	issues := validation.New()
	issues.Required("email", request.Email)
	issues.RequiredExact("password", request.Password)
	if !issues.Empty() {
		httpx.ErrorWithDetails(c, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "One or more fields are invalid.", httpx.ErrorItems(issues.Issues()))
		return
	}

	session, err := h.service.Login(c.Request.Context(), service.LoginInput{
		Email:     request.Email,
		Password:  request.Password,
		UserAgent: c.Request.UserAgent(),
		IP:        clientAddress(c),
	})
	if err != nil {
		h.fail(c, err)
		return
	}

	h.cookies.SetSession(c, session)
	account := accountBody(session.Account)
	httpx.Success(c, http.StatusOK, sessionBody(session, &account, tokenMode(c)))
}

// refresh handles POST /auth/refresh (API v1.1 section 2.3). The refresh token
// comes from the cookie, or from the body in token mode.
func (h *authHandlers) refresh(c *gin.Context) {
	token := h.cookies.RefreshToken(c)
	if tokenMode(c) {
		var request refreshRequest
		if !bindJSON(c, &request) {
			return
		}
		token = request.RefreshToken
	}

	session, err := h.service.Refresh(c.Request.Context(), service.RefreshInput{
		RefreshToken: token,
		UserAgent:    c.Request.UserAgent(),
		IP:           clientAddress(c),
	})
	if err != nil {
		if errors.Is(err, service.ErrRefreshTokenInvalid) {
			// A dead token must not linger in the browser.
			h.cookies.Clear(c)
		}
		h.fail(c, err)
		return
	}

	h.cookies.SetSession(c, session)
	httpx.Success(c, http.StatusOK, sessionBody(session, nil, tokenMode(c)))
}

// logout handles POST /auth/logout (API v1.1 section 2.4). The caller is
// already authenticated by the access token.
func (h *authHandlers) logout(c *gin.Context) {
	claims, ok := auth.ClaimsFrom(c)
	if !ok {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}

	token := h.cookies.RefreshToken(c)
	if token == "" && tokenMode(c) && c.Request.ContentLength != 0 {
		var request refreshRequest
		if !bindJSON(c, &request) {
			return
		}
		token = request.RefreshToken
	}

	if err := h.service.Logout(c.Request.Context(), claims.UserID, token); err != nil {
		h.fail(c, err)
		return
	}
	h.cookies.Clear(c)
	c.Status(http.StatusNoContent)
}

// fail maps a service error to its API v1.1 response. Unexpected errors are
// logged once here and answered with a generic 500.
func (h *authHandlers) fail(c *gin.Context, err error) {
	var validationErr *service.ValidationError
	switch {
	case errors.As(err, &validationErr):
		httpx.ErrorWithDetails(c, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "One or more fields are invalid.", httpx.ErrorItems(validationErr.Issues))
	case errors.Is(err, service.ErrEmailAlreadyExists):
		httpx.Error(c, http.StatusConflict, "EMAIL_ALREADY_EXISTS", "an account with this email already exists")
	case errors.Is(err, service.ErrInvalidCredentials):
		httpx.Error(c, http.StatusUnauthorized, "INVALID_CREDENTIALS", "invalid email or password")
	case errors.Is(err, service.ErrEmailVerificationRequired):
		httpx.Error(c, http.StatusForbidden, "EMAIL_VERIFICATION_REQUIRED", "verify your email address before signing in")
	case errors.Is(err, service.ErrVerificationTokenInvalid):
		httpx.Error(c, http.StatusBadRequest, "INVALID_OR_EXPIRED_VERIFICATION_TOKEN", "the verification link is invalid or has expired")
	case errors.Is(err, service.ErrMerchantAccountInactive):
		var inactive *service.MerchantInactiveError
		if errors.As(err, &inactive) {
			httpx.ErrorWithDetails(c, http.StatusForbidden, "MERCHANT_ACCOUNT_INACTIVE", "this merchant account is not active", merchantInactiveDetails(inactive))
			return
		}
		httpx.Error(c, http.StatusForbidden, "MERCHANT_ACCOUNT_INACTIVE", "this merchant account is not active")
	case errors.Is(err, service.ErrServiceUnavailable):
		httpx.Error(c, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "a required service is unavailable, try again later")
	case errors.Is(err, service.ErrAccountDisabled):
		httpx.Error(c, http.StatusForbidden, "ACCOUNT_DISABLED", "this account has been disabled")
	case errors.Is(err, service.ErrAccountLocked):
		httpx.Error(c, http.StatusLocked, "ACCOUNT_LOCKED", "too many failed attempts, try again later")
	case errors.Is(err, service.ErrRefreshTokenInvalid):
		httpx.Error(c, http.StatusUnauthorized, "REFRESH_TOKEN_INVALID", "the session has expired, please sign in again")
	default:
		h.logger.ErrorContext(c.Request.Context(), "auth request failed",
			"request_id", requestid.FromGin(c),
			"path", c.FullPath(),
			"error", err.Error(),
		)
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
	}
}

// merchantInactiveDetails lists what a merchant owner may be told after the
// correct password: the application status, the administrator's reason when
// there is one and the support address when one is configured. They use the
// API's field and issue pairs, so no new error shape is needed.
func merchantInactiveDetails(inactive *service.MerchantInactiveError) []httpx.ErrorItem {
	items := []httpx.ErrorItem{{Field: "status", Issue: inactive.Status}}
	if inactive.Reason != "" {
		items = append(items, httpx.ErrorItem{Field: "reason", Issue: inactive.Reason})
	}
	if inactive.SupportEmail != "" {
		items = append(items, httpx.ErrorItem{Field: "supportEmail", Issue: inactive.SupportEmail})
	}
	return items
}

// bindJSON reads a bounded JSON body into destination. It answers the client
// itself and returns false when the body is unusable.
func bindJSON(c *gin.Context, destination any) bool {
	if c.ContentType() != "application/json" {
		httpx.Error(c, http.StatusUnsupportedMediaType, "UNSUPPORTED_MEDIA_TYPE", "content type must be application/json")
		return false
	}

	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBodyBytes)
	if err := c.ShouldBindJSON(destination); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			httpx.Error(c, http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE", "request body is too large")
			return false
		}
		httpx.Error(c, http.StatusBadRequest, "BAD_REQUEST", "request body must be valid JSON")
		return false
	}
	return true
}

// tokenMode reports whether the caller asked for tokens in the response body,
// as API tools such as Postman do (API v1.1 section 1.2).
func tokenMode(c *gin.Context) bool {
	return strings.EqualFold(c.GetHeader(tokenModeHeader), tokenModeBody)
}

func clientAddress(c *gin.Context) netip.Addr {
	address, err := netip.ParseAddr(c.ClientIP())
	if err != nil {
		return netip.Addr{}
	}
	return address
}

func accountBody(account service.Account) accountResponse {
	body := accountResponse{
		ID:            account.ID,
		Email:         account.Email,
		FullName:      account.FullName,
		Phone:         account.Phone,
		Role:          account.Role,
		EmailVerified: account.EmailVerified,
	}
	if !account.CreatedAt.IsZero() {
		body.CreatedAt = account.CreatedAt.UTC().Format(time.RFC3339)
	}
	return body
}

// sessionBody builds the login or refresh payload. Tokens appear in the body
// only in token mode; browsers receive them as HttpOnly cookies only.
func sessionBody(session service.Session, user *accountResponse, withTokens bool) sessionResponse {
	body := sessionResponse{User: user, ExpiresIn: session.ExpiresIn}
	if user != nil {
		// Login returns only the fields the API lists for the signed-in user.
		trimmed := *user
		trimmed.Phone, trimmed.CreatedAt = "", ""
		body.User = &trimmed
	}
	if withTokens {
		body.AccessToken = session.AccessToken
		body.RefreshToken = session.RefreshToken
	}
	return body
}
