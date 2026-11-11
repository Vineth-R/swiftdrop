package servicehttp

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/repository"
	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
	"github.com/Vineth-R/swiftdrop/packages/go-common/httpx"
	"github.com/Vineth-R/swiftdrop/packages/go-common/requestid"
	"github.com/Vineth-R/swiftdrop/packages/go-common/secretbox"
	"github.com/Vineth-R/swiftdrop/packages/go-common/webhookurl"
	"github.com/Vineth-R/swiftdrop/packages/webhookdelivery"
)

// This file implements the merchant webhook settings endpoints, API v1.1
// sections 8.12 to 8.15. The database code is in repository/webhook.go and the
// signed sending is in packages/webhookdelivery.
//
// The endpoints are session-authenticated: the caller's HS256 access token
// (cookie or bearer) must carry the MERCHANT role, and the merchant record is
// resolved from the verified token subject through merchant.merchants
// .owner_user_id. A merchant ID is never read from the request, so one merchant
// cannot act on another merchant's settings.
//
// Fail-closed configuration:
//   - without WEBHOOK_SECRET_KEY the routes answer 503 SERVICE_UNAVAILABLE;
//   - without JWT_ACCESS_SECRET the routes answer 503 SERVICE_UNAVAILABLE.
//
// Production dependency: merchant onboarding (creating a MERCHANT identity user
// and its merchant record) is still owned by a later milestone. Until it lands,
// these endpoints remain unavailable in practice even though authentication is
// enforced.

const (
	defaultPage      = 1
	defaultPageSize  = 20
	maxPage          = 10_000 // matches the notification-log list limit
	maxPageSize      = 100
	maxTrackingChars = 20
)

// The small interfaces below let tests replace the database, the encryption
// and the real HTTP sender. *repository.Repository, *secretbox.Box and
// *webhookdelivery.Sender satisfy them.
type webhookStore interface {
	MerchantByOwnerUserID(ctx context.Context, ownerUserID string) (repository.Merchant, error)
	WebhookByMerchant(ctx context.Context, merchantID string) (repository.Webhook, error)
	SaveWebhook(ctx context.Context, p repository.SaveWebhookParams) (repository.WebhookSaveResult, error)
	ListDeliveryAttempts(ctx context.Context, merchantID string, f repository.DeliveryFilter) ([]repository.DeliveryAttempt, int, error)
}

type secretCipher interface {
	Encrypt(plaintext string) (string, error)
	Decrypt(stored string) (string, error)
}

type webhookSender interface {
	Send(ctx context.Context, target webhookdelivery.Target, event webhookdelivery.Event) webhookdelivery.Result
}

// WebhookConfig wires the webhook settings routes. A nil Box or an empty
// AccessSecret keeps the routes fail closed with 503.
type WebhookConfig struct {
	Repository   *repository.Repository
	Box          *secretbox.Box
	Sender       *webhookdelivery.Sender
	AccessSecret string
	CookieName   string
	Logger       *slog.Logger
	// SessionCheck returns the current Identity active flag and session
	// version. It is optional so existing tests can exercise the webhook
	// handlers directly.
	SessionCheck func(ctx context.Context, userID string) (bool, int, error)
}

// RegisterWebhookRoutes adds the four webhook endpoints to the router.
func RegisterWebhookRoutes(router *gin.Engine, config WebhookConfig) error {
	// Convert concrete pointers to interfaces only when they are non-nil; a
	// typed nil pointer inside an interface is not nil and would defeat the
	// fail-closed checks below.
	var store webhookStore
	if config.Repository != nil {
		store = config.Repository
	}
	var cipher secretCipher
	if config.Box != nil {
		cipher = config.Box
	}
	var sender webhookSender
	if config.Sender != nil {
		sender = config.Sender
	}
	_, err := registerWebhookRoutes(router, store, cipher, sender,
		config.AccessSecret, config.CookieName, config.SessionCheck, config.Logger)
	return err
}

// urlChecker decides whether a merchant URL may be saved.
type urlChecker func(ctx context.Context, raw string) error

// checkWebhookURL is the production check: the text rules first, then a DNS
// lookup so a name that points at a private address is refused at save time.
// The sender checks the address again at connect time.
func checkWebhookURL(ctx context.Context, raw string) error {
	if err := webhookurl.Validate(raw); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return webhookurl.ValidateResolved(ctx, raw, net.DefaultResolver)
}

type webhookHandler struct {
	store    webhookStore
	cipher   secretCipher
	sender   webhookSender
	checkURL urlChecker
	logger   *slog.Logger
	now      func() time.Time
}

func registerWebhookRoutes(router *gin.Engine, store webhookStore, cipher secretCipher, sender webhookSender,
	accessSecret, cookieName string, sessionCheck func(context.Context, string) (bool, int, error), logger *slog.Logger,
) (*webhookHandler, error) {
	h := &webhookHandler{store: store, cipher: cipher, sender: sender, checkURL: checkWebhookURL, logger: logger, now: time.Now}
	group := router.Group("/merchants/me/webhook", noStore())

	switch {
	case store == nil || cipher == nil:
		group.Use(unavailable("webhook settings are not configured"))
	case accessSecret == "":
		group.Use(unavailable("authentication is not configured"))
	default:
		manager, err := auth.NewTokenManager(accessSecret, time.Minute)
		if err != nil {
			return nil, err
		}
		if cookieName == "" {
			cookieName = "sd_access"
		}
		group.Use(auth.Authenticate(manager, cookieName), auth.RequireRoles(auth.RoleMerchant))
		if sessionCheck != nil {
			group.Use(func(c *gin.Context) {
				claims, ok := auth.ClaimsFrom(c)
				if !ok {
					httpx.Error(c, http.StatusUnauthorized, "UNAUTHENTICATED", "authentication required")
					return
				}
				active, version, err := sessionCheck(c.Request.Context(), claims.UserID)
				if err != nil {
					httpx.Error(c, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "authentication could not be verified")
					return
				}
				if !active || version != claims.SessionVersion {
					httpx.Error(c, http.StatusUnauthorized, "SESSION_REVOKED", "this session is no longer valid")
					return
				}
				c.Next()
			})
		}
	}

	group.GET("", h.get)
	group.PUT("", h.put)
	group.POST("/test", h.test)
	group.GET("/deliveries", h.deliveries)
	return h, nil
}

func noStore() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		c.Next()
	}
}

func unavailable(message string) gin.HandlerFunc {
	return func(c *gin.Context) {
		httpx.Error(c, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", message)
		c.Abort()
	}
}

// merchant resolves the authenticated caller's own merchant record from the
// verified token subject. It never trusts a merchant ID supplied by the client.
func (h *webhookHandler) merchant(c *gin.Context) (repository.Merchant, bool) {
	claims, ok := auth.ClaimsFrom(c)
	if !ok {
		httpx.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "merchant authentication required")
		return repository.Merchant{}, false
	}
	merchant, err := h.store.MerchantByOwnerUserID(c.Request.Context(), claims.UserID)
	if errors.Is(err, repository.ErrNotFound) {
		httpx.Error(c, http.StatusForbidden, "FORBIDDEN", "merchant account is not provisioned")
		return repository.Merchant{}, false
	}
	if err != nil {
		h.internalError(c, "merchant_lookup", "webhook operation failed", err)
		return repository.Merchant{}, false
	}
	if merchant.Status != repository.MerchantStatusApproved {
		httpx.Error(c, http.StatusForbidden, "MERCHANT_NOT_APPROVED", "merchant approval is required")
		return repository.Merchant{}, false
	}
	return merchant, true
}

func webhookView(w repository.Webhook) gin.H {
	types := w.EventTypes
	if types == nil {
		types = []string{}
	}
	return gin.H{"url": w.URL, "eventTypes": types, "isActive": w.IsActive, "updatedAt": w.UpdatedAt}
}

// get implements 8.12. It never returns the signing secret.
func (h *webhookHandler) get(c *gin.Context) {
	merchant, ok := h.merchant(c)
	if !ok {
		return
	}
	webhook, err := h.store.WebhookByMerchant(c.Request.Context(), merchant.ID)
	if err != nil {
		h.webhookError(c, "get_webhook", err)
		return
	}
	httpx.Success(c, http.StatusOK, webhookView(webhook))
}

type webhookRequest struct {
	URL          string   `json:"url"`
	EventTypes   []string `json:"eventTypes"`
	IsActive     *bool    `json:"isActive"`
	RotateSecret bool     `json:"rotateSecret"`
}

// normalizeEventTypes applies the default and accepts only known event types.
func normalizeEventTypes(in []string) ([]string, bool) {
	if len(in) == 0 {
		return []string{webhookdelivery.EventTypeStatusChanged}, true
	}
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, eventType := range in {
		if eventType != webhookdelivery.EventTypeStatusChanged {
			return nil, false
		}
		if !seen[eventType] {
			seen[eventType] = true
			out = append(out, eventType)
		}
	}
	return out, true
}

// put implements 8.13: create or replace the webhook, with optional rotation.
func (h *webhookHandler) put(c *gin.Context) {
	merchant, ok := h.merchant(c)
	if !ok {
		return
	}
	if merchant.Status != repository.MerchantStatusApproved {
		httpx.Error(c, http.StatusForbidden, "MERCHANT_NOT_APPROVED", "merchant approval is required")
		return
	}
	ctx := c.Request.Context()

	var request webhookRequest
	if c.ShouldBindJSON(&request) != nil {
		httpx.Error(c, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "invalid webhook settings")
		return
	}
	rawURL := strings.TrimSpace(request.URL)
	if err := h.checkURL(ctx, rawURL); err != nil {
		switch {
		case errors.Is(err, webhookurl.ErrEmpty), errors.Is(err, webhookurl.ErrTooLong), errors.Is(err, webhookurl.ErrInvalid):
			httpx.Error(c, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "a valid webhook url is required")
		default:
			// Same message for every reason, so the response does not help
			// someone probe internal addresses.
			httpx.Error(c, http.StatusUnprocessableEntity, "WEBHOOK_URL_NOT_ALLOWED", "the webhook url must be a public https address")
		}
		return
	}
	eventTypes, valid := normalizeEventTypes(request.EventTypes)
	if !valid {
		httpx.Error(c, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "unsupported event type")
		return
	}
	isActive := true
	if request.IsActive != nil {
		isActive = *request.IsActive
	}

	// A candidate secret is always prepared. The repository stores it only on
	// insert or rotation and reports whether it did; a concurrent first save
	// that loses the race keeps the winner's secret and must not return this
	// candidate.
	secret, err := secretbox.GenerateSecret()
	if err != nil {
		h.internalError(c, "generate_webhook_secret", "could not generate signing secret", err)
		return
	}
	encrypted, err := h.cipher.Encrypt(secret)
	if err != nil {
		h.internalError(c, "encrypt_webhook_secret", "could not protect signing secret", err)
		return
	}
	id, err := randomUUID()
	if err != nil {
		h.internalError(c, "generate_webhook_id", "could not generate webhook identifier", err)
		return
	}

	result, err := h.store.SaveWebhook(ctx, repository.SaveWebhookParams{
		ID:                 id,
		MerchantID:         merchant.ID,
		URL:                rawURL,
		EventTypes:         eventTypes,
		IsActive:           isActive,
		RotateSecret:       request.RotateSecret,
		CandidateSecretEnc: []byte(encrypted),
	})
	if err != nil {
		h.webhookError(c, "save_webhook", err)
		return
	}

	view := webhookView(result.Webhook)
	if result.SecretStored {
		view["signingSecret"] = secret // shown once, never stored in clear
	}
	httpx.Success(c, http.StatusOK, view)
}

// test implements 8.14: send a sample event and report what happened.
func (h *webhookHandler) test(c *gin.Context) {
	merchant, ok := h.merchant(c)
	if !ok {
		return
	}
	ctx := c.Request.Context()
	webhook, err := h.store.WebhookByMerchant(ctx, merchant.ID)
	if err != nil {
		h.webhookError(c, "get_webhook", err)
		return
	}
	secret, err := h.cipher.Decrypt(string(webhook.SigningSecretEnc))
	if err != nil {
		h.internalError(c, "decrypt_webhook_secret", "could not read signing secret", err)
		return
	}
	eventID, err := randomUUID()
	if err != nil {
		h.internalError(c, "generate_event_id", "could not generate event identifier", err)
		return
	}
	result := h.sender.Send(ctx, webhookdelivery.Target{URL: webhook.URL, Secret: secret},
		webhookdelivery.SampleEvent(eventID, h.now()))

	var httpStatus any // null when no response arrived (timeout, connection error)
	if result.StatusCode != 0 {
		httpStatus = result.StatusCode
	}
	httpx.Success(c, http.StatusOK, gin.H{
		"delivered":  result.Success,
		"httpStatus": httpStatus,
		"durationMs": result.Duration.Milliseconds(),
	})
}

// positiveInt reads a query value that must be a whole number of at least 1.
func positiveInt(raw string, fallback int) (int, bool) {
	if raw == "" {
		return fallback, true
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 1 {
		return 0, false
	}
	return value, true
}

// deliveries implements 8.15: the attempt log with filters and paging.
func (h *webhookHandler) deliveries(c *gin.Context) {
	merchant, ok := h.merchant(c)
	if !ok {
		return
	}
	page, pageOK := positiveInt(c.Query("page"), defaultPage)
	pageSize, sizeOK := positiveInt(c.Query("pageSize"), defaultPageSize)
	if !pageOK || !sizeOK {
		httpx.Error(c, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "page and pageSize must be positive numbers")
		return
	}
	if page > maxPage {
		httpx.Error(c, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "page must be at most "+strconv.Itoa(maxPage))
		return
	}
	if pageSize > maxPageSize {
		pageSize = maxPageSize
	}

	filter := repository.DeliveryFilter{
		TrackingNumber: strings.TrimSpace(c.Query("trackingNumber")),
		Limit:          pageSize,
		// page <= maxPage and pageSize <= maxPageSize, so the offset is
		// positive and cannot overflow.
		Offset: (page - 1) * pageSize,
	}
	switch c.Query("success") {
	case "":
	case "true":
		value := true
		filter.Success = &value
	case "false":
		value := false
		filter.Success = &value
	default:
		httpx.Error(c, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "success must be true or false")
		return
	}
	if len(filter.TrackingNumber) > maxTrackingChars {
		httpx.Error(c, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "invalid tracking number")
		return
	}

	attempts, total, err := h.store.ListDeliveryAttempts(c.Request.Context(), merchant.ID, filter)
	if err != nil {
		h.webhookError(c, "list_deliveries", err)
		return
	}
	data := make([]gin.H, 0, len(attempts))
	for _, a := range attempts {
		data = append(data, gin.H{
			"eventId":        a.EventID,
			"trackingNumber": a.TrackingNumber,
			"attemptNumber":  a.AttemptNumber,
			"httpStatus":     a.HTTPStatus,
			"success":        a.Success,
			"nextRetryAt":    a.NextRetryAt,
			"deadLettered":   a.DeadLettered,
			"createdAt":      a.CreatedAt,
		})
	}
	httpx.SuccessList(c, http.StatusOK, data, httpx.NewPagination(page, pageSize, total))
}

func (h *webhookHandler) webhookError(c *gin.Context, operation string, err error) {
	switch {
	case errors.Is(err, repository.ErrWebhookNotConfigured):
		httpx.Error(c, http.StatusNotFound, "WEBHOOK_NOT_CONFIGURED", "no webhook is configured")
	case errors.Is(err, repository.ErrNotFound):
		httpx.Error(c, http.StatusNotFound, "NOT_FOUND", "merchant not found")
	default:
		h.internalError(c, operation, "webhook operation failed", err)
	}
}

func (h *webhookHandler) internalError(c *gin.Context, operation, message string, err error) {
	if h.logger != nil {
		// Only the error text is logged. URLs, secrets and bodies are never added.
		h.logger.ErrorContext(c.Request.Context(), "webhook request failed",
			"request_id", requestid.FromGin(c),
			"path", c.FullPath(),
			"operation", operation,
			"error", err.Error(),
		)
	}
	httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", message)
}
