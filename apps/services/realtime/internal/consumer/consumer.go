// Package consumer turns parcel.status_changed.v1 broker events into live
// updates for connected WebSocket clients.
package consumer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/Vineth-R/swiftdrop/apps/services/realtime/internal/live"
	"github.com/Vineth-R/swiftdrop/packages/go-common/messaging"
)

// EventType is the only event this consumer understands.
const EventType = "parcel.status_changed.v1"

// Broadcaster delivers an update to the clients following it. *live.Hub
// satisfies it.
type Broadcaster interface {
	Broadcast(update live.Update) int
}

// Handler implements messaging.Handler.
type Handler struct {
	hub Broadcaster
}

func New(hub Broadcaster) *Handler {
	return &Handler{hub: hub}
}

// payload lists only the fields API v1.1 section 9 lets clients see. Anything
// else in the event (merchant_id, customer_user_id, parcel_id, note,
// failure_reason, ...) is never decoded, so it cannot be forwarded.
type payload struct {
	TrackingNumber string `json:"tracking_number"`
	PreviousStatus string `json:"previous_status"`
	Status         string `json:"status"`
}

// Handle decodes one message. Malformed or unexpected events are permanent
// errors: retrying cannot fix them.
func (h *Handler) Handle(_ context.Context, message messaging.Message) error {
	var envelope messaging.Envelope
	if err := json.NewDecoder(bytes.NewReader(message.Body)).Decode(&envelope); err != nil {
		return messaging.Permanent(fmt.Errorf("decode envelope: %w", err))
	}
	if err := envelope.Validate(); err != nil {
		return messaging.Permanent(fmt.Errorf("invalid envelope: %w", err))
	}
	if envelope.Type != EventType {
		return messaging.Permanent(fmt.Errorf("unexpected event type %q", envelope.Type))
	}

	var data payload
	if err := json.Unmarshal(envelope.Data, &data); err != nil {
		return messaging.Permanent(fmt.Errorf("decode data: %w", err))
	}
	if !live.ValidTrackingNumber(data.TrackingNumber) {
		return messaging.Permanent(fmt.Errorf("invalid tracking_number %q", data.TrackingNumber))
	}
	if !live.ValidStatus(data.Status) {
		return messaging.Permanent(fmt.Errorf("invalid status %q", data.Status))
	}
	if !live.ValidStatus(data.PreviousStatus) {
		return messaging.Permanent(fmt.Errorf("invalid previous_status %q", data.PreviousStatus))
	}

	h.hub.Broadcast(live.Update{
		TrackingNumber: data.TrackingNumber,
		PreviousStatus: data.PreviousStatus,
		Status:         data.Status,
		UpdatedAt:      envelope.OccurredAt,
	})
	return nil
}
