package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/Vineth-R/swiftdrop/packages/go-common/messaging"
)

// EventType is the event published by the parcel service when a parcel changes status.
const EventType = "parcel.status_changed.v1"

type Handler struct {
	logger *slog.Logger
}

func New(logger *slog.Logger) *Handler {
	return &Handler{logger: logger}
}

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

	h.logger.InfoContext(ctx, "webhook delivery event received",
		"event_id", envelope.ID,
		"event_type", envelope.Type,
		"correlation_id", envelope.CorrelationID,
	)

	return nil
}
