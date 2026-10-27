package messaging

import (
	"encoding/json"
	"errors"
	"strings"
	"time"
)

type Envelope struct {
	ID            string          `json:"event_id"`
	Type          string          `json:"event_type"`
	Version       int             `json:"event_version"`
	OccurredAt    time.Time       `json:"occurred_at"`
	Producer      string          `json:"producer"`
	CorrelationID string          `json:"correlation_id,omitempty"`
	Data          json.RawMessage `json:"data"`
}

func (e Envelope) Validate() error {
	if strings.TrimSpace(e.ID) == "" {
		return errors.New("event_id is required")
	}
	if strings.TrimSpace(e.Type) == "" {
		return errors.New("event_type is required")
	}
	if e.Version < 1 {
		return errors.New("event_version must be at least 1")
	}
	if e.OccurredAt.IsZero() {
		return errors.New("occurred_at is required")
	}
	if strings.TrimSpace(e.Producer) == "" {
		return errors.New("producer is required")
	}
	if len(e.Data) == 0 || !json.Valid(e.Data) {
		return errors.New("data must be valid JSON")
	}
	return nil
}
