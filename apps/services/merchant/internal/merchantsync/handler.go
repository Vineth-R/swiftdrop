// Package merchantsync consumes parcel.status_changed.v1 and keeps the
// merchant order snapshot and timeline in sync.
package merchantsync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/orders"
	"github.com/Vineth-R/swiftdrop/packages/go-common/messaging"
	"github.com/Vineth-R/swiftdrop/packages/go-common/validation"
)

const eventType = "parcel.status_changed.v1"

// Store is the merchant-owned persistence seam for status synchronization.
type Store interface {
	ApplyStatusEvent(ctx context.Context, event orders.StatusEvent) (bool, error)
}

// Handler processes parcel status events for merchant orders.
type Handler struct {
	store  Store
	logger *slog.Logger
}

// New returns a status synchronization handler.
func New(store Store, logger *slog.Logger) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{store: store, logger: logger}
}

type payload struct {
	ParcelID       string `json:"parcel_id"`
	TrackingNumber string `json:"tracking_number"`
	PreviousStatus string `json:"previous_status"`
	Status         string `json:"status"`
	MerchantID     string `json:"merchant_id"`
	FailureReason  string `json:"failure_reason"`
}

// Handle validates one event and applies it exactly once. Malformed events are
// permanent failures; repository outages stay retryable.
func (h *Handler) Handle(ctx context.Context, message messaging.Message) error {
	if err := messaging.ValidateMessage(message.ContentType, message.Body); err != nil {
		return messaging.Permanent(fmt.Errorf("validate merchant sync message: %w", err))
	}
	var envelope messaging.Envelope
	if err := json.Unmarshal(message.Body, &envelope); err != nil {
		return messaging.Permanent(fmt.Errorf("decode merchant sync envelope: %w", err))
	}
	if err := envelope.Validate(); err != nil {
		return messaging.Permanent(fmt.Errorf("validate merchant sync envelope: %w", err))
	}
	if envelope.Type != eventType {
		return messaging.Permanent(fmt.Errorf("unsupported event type %q", envelope.Type))
	}
	if envelope.Version != 1 {
		return messaging.Permanent(fmt.Errorf("unsupported event version %d", envelope.Version))
	}
	if envelope.Producer != "parcel" {
		return messaging.Permanent(fmt.Errorf("unsupported event producer %q", envelope.Producer))
	}
	if !validation.UUID(envelope.ID) {
		return messaging.Permanent(errors.New("event_id must be a UUID"))
	}

	var data payload
	if err := json.Unmarshal(envelope.Data, &data); err != nil {
		return messaging.Permanent(fmt.Errorf("decode merchant sync payload: %w", err))
	}
	data.ParcelID = strings.ToLower(strings.TrimSpace(data.ParcelID))
	data.MerchantID = strings.ToLower(strings.TrimSpace(data.MerchantID))
	data.TrackingNumber = strings.TrimSpace(data.TrackingNumber)
	data.PreviousStatus = strings.TrimSpace(data.PreviousStatus)
	data.Status = strings.TrimSpace(data.Status)
	data.FailureReason = strings.TrimSpace(data.FailureReason)
	switch {
	case !validation.UUID(data.ParcelID):
		return messaging.Permanent(errors.New("data.parcel_id must be a UUID"))
	case data.MerchantID != "" && !validation.UUID(data.MerchantID):
		return messaging.Permanent(errors.New("data.merchant_id must be a UUID"))
	case validation.RuneLength(data.TrackingNumber) < 11 || validation.RuneLength(data.TrackingNumber) > 20:
		return messaging.Permanent(errors.New("data.tracking_number is invalid"))
	case !orders.ValidStatus(data.PreviousStatus):
		return messaging.Permanent(errors.New("data.previous_status is invalid"))
	case !orders.ValidStatus(data.Status):
		return messaging.Permanent(errors.New("data.status is invalid"))
	case data.Status == "FAILED" && (validation.RuneLength(data.FailureReason) < 5 || validation.RuneLength(data.FailureReason) > 255):
		return messaging.Permanent(errors.New("data.failure_reason is invalid for FAILED"))
	}

	// The queue is bound to every parcel status event. Customer parcels have no
	// merchant_id and are not merchant sync work.
	if data.MerchantID == "" {
		h.logger.DebugContext(ctx, "ignoring non-merchant parcel status event", "event_id", envelope.ID, "parcel_id", data.ParcelID)
		return nil
	}

	applied, err := h.store.ApplyStatusEvent(ctx, orders.StatusEvent{
		EventID:        envelope.ID,
		MerchantID:     data.MerchantID,
		ParcelID:       data.ParcelID,
		TrackingNumber: data.TrackingNumber,
		PreviousStatus: data.PreviousStatus,
		Status:         data.Status,
		OccurredAt:     envelope.OccurredAt.UTC(),
		CorrelationID:  envelope.CorrelationID,
	})
	switch {
	case errors.Is(err, orders.ErrTrackingMismatch):
		return messaging.Permanent(fmt.Errorf("merchant order tracking number does not match event: %w", err))
	case errors.Is(err, orders.ErrNotFound):
		// Merchant parcels may exist without a merchant order (for example a
		// trusted internal caller). This is not applicable work, not a poison
		// message, so acknowledge it and leave a warning for operators.
		h.logger.WarnContext(ctx, "merchant order not found for status event",
			"event_id", envelope.ID, "parcel_id", data.ParcelID, "merchant_id", data.MerchantID)
		return nil
	case err != nil:
		return fmt.Errorf("apply merchant status event: %w", err)
	}
	if applied {
		h.logger.InfoContext(ctx, "merchant order status synchronized",
			"event_id", envelope.ID, "parcel_id", data.ParcelID, "merchant_id", data.MerchantID,
			"status", data.Status, "correlation_id", envelope.CorrelationID)
	} else {
		h.logger.DebugContext(ctx, "merchant status event ignored",
			"event_id", envelope.ID, "parcel_id", data.ParcelID, "status", data.Status)
	}
	return nil
}
