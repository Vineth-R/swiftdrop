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
	"github.com/Vineth-R/swiftdrop/packages/go-common/httpx"
	"github.com/Vineth-R/swiftdrop/packages/go-common/requestid"
	"github.com/Vineth-R/swiftdrop/packages/go-common/secretbox"
	"github.com/Vineth-R/swiftdrop/packages/go-common/webhookdelivery"
	"github.com/Vineth-R/swiftdrop/packages/go-common/webhookurl"
)

// This file implements the merchant webhook settings endpoints, API v1.1
// sections 8.12 to 8.15. The database code is in repository/webhook.go and the
// signed sending is in packages/go-common/webhookdelivery.

// The small interfaces below let tests replace the database, the encryption
// and the real HTTP sender. *repository.Repository, *secretbox.Box and
// *webhookdelivery.Sender satisfy them.
type webhookStore interface {
	MerchantByID(ctx context.Context, id string) (repository.Merchant, error)
	WebhookByMerchant(ctx context.Context, merchantID string) (repository.Webhook, error)
	SaveWebhook(ctx context.Context, p repository.SaveWebhookParams) (repository.Webhook, error)
	ListDeliveryAttempts(ctx context.Context, merchantID string, f repository.DeliveryFilter) ([]repository.DeliveryAttempt, int, error)
}

type secretCipher interface {
	Encrypt(plaintext string) (string, error)
	Decrypt(stored string) (string, error)
}

type webhookSender interface {
	Send(ctx context.Context, target webhookdelivery.Target, event webhookdelivery.Event) webhookdelivery.Result
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

// RegisterWebhookRoutes adds the four webhook endpoints to the router.
func RegisterWebhookRoutes(router *gin.Engine, repo *repository.Repository, box *secretbox.Box, sender *webhookdelivery.Sender, logger *slog.Logger) {
	registerWebhookRoutes(router, repo, box, sender, logger)
}

func registerWebhookRoutes(router *gin.Engine, store webhookStore, cipher secretCipher, sender webhookSender, logger *slog.Logger) *webhookHandler {
	h := &webhookHandler{store: store, cipher: cipher, sender: sender, checkURL: checkWebhookURL, logger: logger, now: time.Now}
	group := router.Group("/merchants/me/webhook")
	group.GET("", h.get)
	group.PUT("", h.put)
	group.POST("/test", h.test)
	group.GET("/deliveries", h.deliveries)
	return h
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
	identity, ok := merchantIdentity(c)
	if !ok {
		return
	}
	webhook, err := h.store.WebhookByMerchant(c.Request.Context(), identity.MerchantID)
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
	identity, ok := merchantIdentity(c)
	if !ok {
		return
	}
	ctx := c.Request.Context()

	merchant, err := h.store.MerchantByID(ctx, identity.MerchantID)
	if err != nil {
		h.webhookError(c, "merchant_lookup", err)
		return
	}
	if merchant.Status != repository.MerchantStatusApproved {
		httpx.Error(c, http.StatusForbidden, "MERCHANT_NOT_APPROVED", "merchant approval is required")
		return
	}

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

	existing, err := h.store.WebhookByMerchant(ctx, identity.MerchantID)
	configured := err == nil
	if err != nil && !errors.Is(err, repository.ErrWebhookNotConfigured) {
		h.webhookError(c, "get_webhook", err)
		return
	}

	params := repository.SaveWebhookParams{
		MerchantID: identity.MerchantID,
		URL:        rawURL,
		EventTypes: eventTypes,
		IsActive:   isActive,
	}
	newSecret := ""
	if !configured || request.RotateSecret {
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
		newSecret = secret
		params.SecretEnc = []byte(encrypted)
		params.ReplaceSecret = true
	} else {
		params.SecretEnc = existing.SigningSecretEnc
	}
	id, err := randomUUID()
	if err != nil {
		h.internalError(c, "generate_webhook_id", "could not generate webhook identifier", err)
		return
	}
	params.ID = id

	saved, err := h.store.SaveWebhook(ctx, params)
	if err != nil {
		h.webhookError(c, "save_webhook", err)
		return
	}
	view := webhookView(saved)
	if newSecret != "" {
		view["signingSecret"] = newSecret // shown once, never stored in clear
	}
	httpx.Success(c, http.StatusOK, view)
}

// test implements 8.14: send a sample event and report what happened.
func (h *webhookHandler) test(c *gin.Context) {
	identity, ok := merchantIdentity(c)
	if !ok {
		return
	}
	ctx := c.Request.Context()
	webhook, err := h.store.WebhookByMerchant(ctx, identity.MerchantID)
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
	identity, ok := merchantIdentity(c)
	if !ok {
		return
	}
	page, pageOK := positiveInt(c.Query("page"), 1)
	pageSize, sizeOK := positiveInt(c.Query("pageSize"), 20)
	if !pageOK || !sizeOK {
		httpx.Error(c, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "page and pageSize must be positive numbers")
		return
	}
	if pageSize > 100 {
		pageSize = 100
	}

	filter := repository.DeliveryFilter{
		TrackingNumber: strings.TrimSpace(c.Query("trackingNumber")),
		Limit:          pageSize,
		Offset:         (page - 1) * pageSize,
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
	if len(filter.TrackingNumber) > 20 {
		httpx.Error(c, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "invalid tracking number")
		return
	}

	attempts, total, err := h.store.ListDeliveryAttempts(c.Request.Context(), identity.MerchantID, filter)
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
