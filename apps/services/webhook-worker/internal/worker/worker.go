package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/Vineth-R/swiftdrop/packages/go-common/messaging"
	"github.com/Vineth-R/swiftdrop/packages/webhookdelivery"
)

// EventType is the event published by the parcel service when a parcel changes status.
const EventType = "parcel.status_changed.v1"

// maxDurationMs matches the limit of the internal attempts endpoint.
const maxDurationMs = 600000

// MerchantAPI is what the worker needs from the Merchant service.
type MerchantAPI interface {
	Webhook(ctx context.Context, merchantID string) (Webhook, error)
	Delivered(ctx context.Context, merchantID, eventID string) (bool, error)
	RecordAttempt(ctx context.Context, merchantID string, attempt Attempt) error
}

// ParcelAPI is what the worker needs from the Parcel service.
type ParcelAPI interface {
	ExternalOrderID(ctx context.Context, parcelID string) (*string, error)
}

// Sender performs one signed delivery (packages/webhookdelivery).
type Sender interface {
	Send(ctx context.Context, target webhookdelivery.Target, event webhookdelivery.Event) webhookdelivery.Result
}

// Scheduler publishes the message again for a later attempt.
type Scheduler interface {
	Schedule(ctx context.Context, message messaging.Message, nextAttempt int, delay time.Duration) error
}

// Deps are the collaborators of the Handler. Tests replace them with fakes.
type Deps struct {
	Merchants MerchantAPI
	Parcels   ParcelAPI
	Sender    Sender
	Scheduler Scheduler
	// Now is optional and defaults to time.Now; tests set it.
	Now func() time.Time
}

// Handler delivers one parcel status event to the merchant's webhook.
type Handler struct {
	logger *slog.Logger
	deps   Deps
}

func New(logger *slog.Logger, deps Deps) *Handler {
	if deps.Now == nil {
		deps.Now = time.Now
	}
	return &Handler{logger: logger, deps: deps}
}

// statusData is the canonical data object of parcel.status_changed.v1, as the
// courier service stores it in the outbox.
type statusData struct {
	ParcelID       string  `json:"parcel_id"`
	TrackingNumber string  `json:"tracking_number"`
	PreviousStatus *string `json:"previous_status"`
	Status         string  `json:"status"`
	MerchantID     *string `json:"merchant_id"`
	Note           *string `json:"note"`
	FailureReason  *string `json:"failure_reason"`
}

// Handle makes one delivery attempt for one message.
//
// Failure of the merchant's endpoint is not an error here: it is recorded and
// the next attempt is scheduled (1 min, 5 min, 30 min, 2 h, 6 h; after the 6th
// failure the event is dead lettered). An error is returned only for our own
// infrastructure (Merchant or Parcel service, RabbitMQ), so the shared consumer
// can retry the message. If a failure happens after a send, the retry may send
// the same event again; receivers de-duplicate on eventId, and the attempt
// record is idempotent.
func (h *Handler) Handle(ctx context.Context, message messaging.Message) error {
	var envelope messaging.Envelope
	if err := json.Unmarshal(message.Body, &envelope); err != nil {
		return messaging.Permanent(fmt.Errorf("decode event envelope: %w", err))
	}
	if err := envelope.Validate(); err != nil {
		return messaging.Permanent(fmt.Errorf("validate event envelope: %w", err))
	}
	if envelope.Type != EventType {
		return messaging.Permanent(fmt.Errorf("unsupported event type %q", envelope.Type))
	}
	if envelope.Version != 1 {
		return messaging.Permanent(fmt.Errorf("unsupported event version %d", envelope.Version))
	}

	var data statusData
	if err := json.Unmarshal(envelope.Data, &data); err != nil {
		return messaging.Permanent(fmt.Errorf("decode event data: %w", err))
	}
	if data.ParcelID == "" || data.TrackingNumber == "" || data.Status == "" {
		return messaging.Permanent(errors.New("event data needs parcel_id, tracking_number and status"))
	}

	attempt, err := attemptFromHeaders(message.Headers)
	if err != nil {
		return messaging.Permanent(err)
	}
	if attempt < 1 || attempt > webhookdelivery.MaxAttempts {
		return messaging.Permanent(fmt.Errorf("attempt number %d is out of range", attempt))
	}

	logger := h.logger.With("event_id", envelope.ID, "parcel_id", data.ParcelID, "attempt", attempt)

	// A parcel with no merchant (a customer's own parcel) has no webhook.
	if data.MerchantID == nil || strings.TrimSpace(*data.MerchantID) == "" {
		logger.DebugContext(ctx, "event has no merchant, skipping")
		return nil
	}
	merchantID := *data.MerchantID

	webhook, err := h.deps.Merchants.Webhook(ctx, merchantID)
	if errors.Is(err, ErrNotConfigured) {
		logger.WarnContext(ctx, "merchant has no webhook, suppressing delivery", "reason", "webhook_not_configured")
		return nil
	}
	if err != nil {
		return fmt.Errorf("load merchant webhook: %w", err)
	}
	if webhook.MerchantStatus != "APPROVED" {
		reason := "merchant_status_unknown"
		if strings.TrimSpace(webhook.MerchantStatus) != "" {
			reason = "merchant_" + strings.ToLower(webhook.MerchantStatus)
		}
		return h.suppress(ctx, logger, envelope.ID, merchantID, data, attempt, reason)
	}
	if !webhook.IsActive {
		return h.suppress(ctx, logger, envelope.ID, merchantID, data, attempt, "webhook_inactive")
	}
	if !slices.Contains(webhook.EventTypes, webhookdelivery.EventTypeStatusChanged) {
		return h.suppress(ctx, logger, envelope.ID, merchantID, data, attempt, "webhook_not_subscribed")
	}

	// Durable suppression: if this event already recorded a successful
	// delivery, a broker redelivery must not send it again. The check reads the
	// merchant's durable attempt log, so it survives worker restarts.
	delivered, err := h.deps.Merchants.Delivered(ctx, merchantID, envelope.ID)
	if err != nil {
		return fmt.Errorf("check delivered event: %w", err)
	}
	if delivered {
		logger.InfoContext(ctx, "webhook already delivered for event, skipping")
		return nil
	}

	// Looked up before sending, so a Parcel service outage retries without
	// having sent anything.
	externalOrderID, err := h.deps.Parcels.ExternalOrderID(ctx, data.ParcelID)
	if err != nil {
		return fmt.Errorf("load parcel summary: %w", err)
	}

	event := webhookdelivery.Event{
		ID:         envelope.ID,
		Type:       webhookdelivery.EventTypeStatusChanged,
		OccurredAt: envelope.OccurredAt,
		Data: webhookdelivery.EventData{
			TrackingNumber:  data.TrackingNumber,
			ExternalOrderID: externalOrderID,
			PreviousStatus:  data.PreviousStatus,
			Status:          data.Status,
			Note:            data.Note,
			FailureReason:   data.FailureReason,
		},
	}
	result := h.deps.Sender.Send(ctx, webhookdelivery.Target{URL: webhook.URL, Secret: webhook.SigningSecret}, event)

	report := Attempt{
		EventID:        envelope.ID,
		ParcelID:       data.ParcelID,
		TrackingNumber: data.TrackingNumber,
		AttemptNumber:  attempt,
		Success:        result.Success,
		DurationMs:     clampDuration(result.Duration),
	}
	if result.StatusCode >= 100 && result.StatusCode <= 599 {
		status := result.StatusCode
		report.HTTPStatus = &status
	}

	var (
		retryDelay time.Duration
		retry      bool
	)
	if !result.Success {
		report.ErrorMessage = result.ErrorMessage
		if report.ErrorMessage == "" {
			report.ErrorMessage = "unexpected response status"
		}
		retryDelay, retry = webhookdelivery.DelayAfterFailure(attempt)
		if retry {
			next := h.deps.Now().UTC().Add(retryDelay)
			report.NextRetryAt = &next
		} else {
			report.DeadLettered = true
		}
	}

	if err := h.deps.Merchants.RecordAttempt(ctx, merchantID, report); err != nil {
		return fmt.Errorf("record delivery attempt: %w", err)
	}

	switch {
	case result.Success:
		logger.InfoContext(ctx, "webhook delivered", "http_status", result.StatusCode)
	case retry:
		if err := h.deps.Scheduler.Schedule(ctx, message, attempt+1, retryDelay); err != nil {
			return fmt.Errorf("schedule next attempt: %w", err)
		}
		logger.WarnContext(ctx, "webhook delivery failed, retry scheduled",
			"http_status", result.StatusCode, "error", report.ErrorMessage, "retry_in", retryDelay.String())
	default:
		logger.ErrorContext(ctx, "webhook delivery failed, dead lettered",
			"http_status", result.StatusCode, "error", report.ErrorMessage)
	}
	return nil
}

// suppress records why a merchant delivery was not attempted. The event is not
// scheduled for a retry; reinstating a merchant must not replay a suppressed
// delivery unless an operator explicitly requeues it. If the audit record
// cannot be written, the message is dead-lettered rather than retried into a
// later delivery after the merchant is reinstated.
func (h *Handler) suppress(ctx context.Context, logger *slog.Logger, eventID, merchantID string, data statusData, attempt int, reason string) error {
	report := Attempt{
		EventID:        eventID,
		ParcelID:       data.ParcelID,
		TrackingNumber: data.TrackingNumber,
		AttemptNumber:  attempt,
		Success:        false,
		ErrorMessage:   reason,
		DeadLettered:   true,
	}
	if err := h.deps.Merchants.RecordAttempt(ctx, merchantID, report); err != nil {
		logger.ErrorContext(ctx, "suppressed webhook attempt could not be recorded",
			"merchant_id", merchantID, "reason", reason)
		return messaging.Permanent(fmt.Errorf("record suppressed webhook attempt: %w", err))
	}
	logger.WarnContext(ctx, "webhook delivery suppressed", "merchant_id", merchantID, "reason", reason)
	return nil
}

// clampDuration converts to whole milliseconds inside the endpoint's range.
func clampDuration(d time.Duration) int64 {
	ms := d.Milliseconds()
	if ms < 0 {
		return 0
	}
	if ms > maxDurationMs {
		return maxDurationMs
	}
	return ms
}
