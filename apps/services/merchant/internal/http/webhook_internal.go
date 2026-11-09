package servicehttp

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"

	"github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/repository"
	"github.com/Vineth-R/swiftdrop/packages/go-common/httpx"
	"github.com/Vineth-R/swiftdrop/packages/go-common/requestid"
	"github.com/Vineth-R/swiftdrop/packages/go-common/secretbox"
)

// This file implements the two internal endpoints the webhook worker calls on
// the Merchant service (API v1.1 section 13):
//
//	GET  /internal/merchants/{merchantId}/webhook           read URL and signing secret
//	POST /internal/merchants/{merchantId}/webhook/attempts  record one delivery attempt
//
// They are service-to-service only. Nginx never routes /internal, and every call
// must carry X-Internal-Token and a well-formed X-Request-Id. The first GET is
// the only place the plain signing secret ever leaves the Merchant service, so
// the responses are never cached and the secret is never logged.
//
// Fail-closed configuration:
//   - without INTERNAL_TOKEN every call is refused with 401 UNAUTHENTICATED;
//   - without WEBHOOK_SECRET_KEY (or a database) every call that passes the token
//     check answers 503 SERVICE_UNAVAILABLE.

const (
	maxAttemptBodyBytes  = 16 << 10
	maxAttemptNumber     = 6 // API v1.1 section 10.3: six attempts, then dead letter
	maxErrorMessageChars = 255
	maxDurationMS        = 600_000
	uuidTextLength       = 36
)

// internalWebhookStore is the database access the internal endpoints need.
// *repository.Repository satisfies it; tests replace it.
type internalWebhookStore interface {
	WebhookByMerchant(ctx context.Context, merchantID string) (repository.Webhook, error)
	RecordDeliveryAttempt(ctx context.Context, p repository.RecordAttemptParams) (bool, error)
}

// InternalWebhookConfig wires the internal webhook routes. An empty Token
// refuses every call; a nil Box or Repository answers 503.
type InternalWebhookConfig struct {
	Repository *repository.Repository
	Box        *secretbox.Box
	Token      string
	Logger     *slog.Logger
}

// RegisterInternalWebhookRoutes adds the two internal endpoints to the router.
func RegisterInternalWebhookRoutes(router *gin.Engine, config InternalWebhookConfig) {
	// Convert concrete pointers to interfaces only when they are non-nil; a
	// typed nil pointer inside an interface is not nil and would defeat the
	// fail-closed check below.
	var store internalWebhookStore
	if config.Repository != nil {
		store = config.Repository
	}
	var cipher secretCipher
	if config.Box != nil {
		cipher = config.Box
	}
	registerInternalWebhookRoutes(router, store, cipher, config.Token, config.Logger)
}

type internalWebhookHandler struct {
	store  internalWebhookStore
	cipher secretCipher
	logger *slog.Logger
}

func registerInternalWebhookRoutes(router *gin.Engine, store internalWebhookStore, cipher secretCipher,
	token string, logger *slog.Logger,
) *internalWebhookHandler {
	h := &internalWebhookHandler{store: store, cipher: cipher, logger: logger}

	// Order matters: the token is checked first so an unauthenticated caller
	// learns nothing else (not even that the service is misconfigured).
	group := router.Group("/internal/merchants/:merchantId/webhook",
		noStore(), httpx.InternalToken(token), requireInternalRequestID())
	if store == nil || cipher == nil {
		group.Use(unavailable("internal webhook endpoints are not configured"))
	}
	group.GET("", h.get)
	group.POST("/attempts", h.record)
	return h
}

// requireInternalRequestID demands a well-formed X-Request-Id, as the internal
// contract says every call carries one.
func requireInternalRequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !requestid.Valid(c.GetHeader(requestid.Header)) {
			httpx.Error(c, http.StatusBadRequest, "BAD_REQUEST", "a valid X-Request-Id header is required")
			return
		}
		c.Next()
	}
}

// isCanonicalUUID reports whether s is a 36-character hexadecimal UUID. It
// keeps malformed ids away from the database, where the uuid cast would fail.
func isCanonicalUUID(s string) bool {
	if len(s) != uuidTextLength {
		return false
	}
	for i := 0; i < len(s); i++ {
		switch {
		case i == 8 || i == 13 || i == 18 || i == 23:
			if s[i] != '-' {
				return false
			}
		case s[i] >= '0' && s[i] <= '9', s[i] >= 'a' && s[i] <= 'f', s[i] >= 'A' && s[i] <= 'F':
		default:
			return false
		}
	}
	return true
}

// merchantID reads the path id. A malformed id answers 404 like an unknown id,
// the same rule the Identity internal API uses.
func (h *internalWebhookHandler) merchantID(c *gin.Context) (string, bool) {
	id := c.Param("merchantId")
	if !isCanonicalUUID(id) {
		httpx.Error(c, http.StatusNotFound, "NOT_FOUND", "merchant not found")
		return "", false
	}
	return id, true
}

// get implements GET /internal/merchants/{merchantId}/webhook.
func (h *internalWebhookHandler) get(c *gin.Context) {
	merchantID, ok := h.merchantID(c)
	if !ok {
		return
	}
	ctx := c.Request.Context()
	webhook, err := h.store.WebhookByMerchant(ctx, merchantID)
	if err != nil {
		h.fail(c, "get_webhook", err)
		return
	}
	secret, err := h.cipher.Decrypt(string(webhook.SigningSecretEnc))
	if err != nil {
		h.internalError(c, "decrypt_webhook_secret", "could not read signing secret", err)
		return
	}
	types := webhook.EventTypes
	if types == nil {
		types = []string{}
	}
	httpx.Success(c, http.StatusOK, gin.H{
		"url":           webhook.URL,
		"signingSecret": secret,
		"eventTypes":    types,
		"isActive":      webhook.IsActive,
	})
}

// attemptRequest is the worker's report. The fields follow the delivery log in
// API v1.1 section 8.15, plus parcelId (the table requires it), errorMessage
// and durationMs, which the database design (section 6.5) also stores.
//
// Success is a pointer so a missing JSON field is rejected instead of being
// silently treated as false. createdAt is not part of the report: the delivery
// log's createdAt is generated by the Merchant service when it records the
// attempt (see the internal API documentation).
type attemptRequest struct {
	EventID        string     `json:"eventId"`
	ParcelID       string     `json:"parcelId"`
	TrackingNumber string     `json:"trackingNumber"`
	AttemptNumber  int        `json:"attemptNumber"`
	HTTPStatus     *int       `json:"httpStatus"`
	Success        *bool      `json:"success"`
	ErrorMessage   string     `json:"errorMessage"`
	NextRetryAt    *time.Time `json:"nextRetryAt"`
	DeadLettered   bool       `json:"deadLettered"`
	DurationMS     *int       `json:"durationMs"`
}

// validate returns a short reason when the report breaks a rule.
func (r attemptRequest) validate() string {
	switch {
	case !isCanonicalUUID(r.EventID):
		return "eventId must be a UUID"
	case !isCanonicalUUID(r.ParcelID):
		return "parcelId must be a UUID"
	case r.TrackingNumber == "" || len(r.TrackingNumber) > maxTrackingChars:
		return "trackingNumber must be 1 to 20 characters"
	case r.AttemptNumber < 1 || r.AttemptNumber > maxAttemptNumber:
		return "attemptNumber must be 1 to 6"
	case r.Success == nil:
		return "success is required"
	case r.HTTPStatus != nil && (*r.HTTPStatus < 100 || *r.HTTPStatus > 599):
		return "httpStatus must be 100 to 599"
	case r.DurationMS != nil && (*r.DurationMS < 0 || *r.DurationMS > maxDurationMS):
		return "durationMs is out of range"
	case *r.Success && (r.NextRetryAt != nil || r.DeadLettered):
		return "a successful attempt has no retry and is not dead lettered"
	case r.DeadLettered && r.NextRetryAt != nil:
		return "a dead lettered attempt has no next retry"
	}
	return ""
}

// record implements POST /internal/merchants/{merchantId}/webhook/attempts.
// A new attempt answers 201; one that was already recorded answers 200 and
// changes nothing, so the worker can safely repeat a report.
func (h *internalWebhookHandler) record(c *gin.Context) {
	merchantID, ok := h.merchantID(c)
	if !ok {
		return
	}

	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxAttemptBodyBytes)
	var request attemptRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			httpx.Error(c, http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE", "request body is too large")
			return
		}
		httpx.Error(c, http.StatusBadRequest, "BAD_REQUEST", "malformed request body")
		return
	}
	request.TrackingNumber = strings.TrimSpace(request.TrackingNumber)
	if reason := request.validate(); reason != "" {
		httpx.Error(c, http.StatusUnprocessableEntity, "VALIDATION_ERROR", reason)
		return
	}

	id, err := randomUUID()
	if err != nil {
		h.internalError(c, "generate_attempt_id", "could not record the attempt", err)
		return
	}
	params := repository.RecordAttemptParams{
		ID:             id,
		MerchantID:     merchantID,
		EventID:        request.EventID,
		ParcelID:       request.ParcelID,
		TrackingNumber: request.TrackingNumber,
		AttemptNumber:  request.AttemptNumber,
		HTTPStatus:     request.HTTPStatus,
		Success:        *request.Success,
		ErrorMessage:   safeErrorMessage(request.ErrorMessage),
		DeadLettered:   request.DeadLettered,
		DurationMS:     request.DurationMS,
	}
	if request.NextRetryAt != nil {
		retryAt := request.NextRetryAt.UTC()
		params.NextRetryAt = &retryAt
	}

	created, err := h.store.RecordDeliveryAttempt(c.Request.Context(), params)
	if err != nil {
		h.fail(c, "record_attempt", err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	httpx.Success(c, status, gin.H{
		"eventId":       request.EventID,
		"attemptNumber": request.AttemptNumber,
		"recorded":      created,
	})
}

// safeErrorMessage trims the worker's text and cuts it to the column size. An
// empty message is stored as NULL.
func safeErrorMessage(message string) *string {
	message = strings.TrimSpace(message)
	if message == "" {
		return nil
	}
	if utf8.RuneCountInString(message) > maxErrorMessageChars {
		runes := []rune(message)
		message = string(runes[:maxErrorMessageChars])
	}
	return &message
}

func (h *internalWebhookHandler) fail(c *gin.Context, operation string, err error) {
	if errors.Is(err, repository.ErrWebhookNotConfigured) {
		httpx.Error(c, http.StatusNotFound, "WEBHOOK_NOT_CONFIGURED", "no webhook is configured")
		return
	}
	h.internalError(c, operation, "webhook operation failed", err)
}

func (h *internalWebhookHandler) internalError(c *gin.Context, operation, message string, err error) {
	if h.logger != nil {
		// Only the error text is logged. URLs, secrets and bodies are never added.
		h.logger.ErrorContext(c.Request.Context(), "internal webhook request failed",
			"request_id", requestid.FromGin(c),
			"path", c.FullPath(),
			"operation", operation,
			"error", err.Error(),
		)
	}
	httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", message)
}
