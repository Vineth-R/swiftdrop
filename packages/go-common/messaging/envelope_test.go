package messaging

import (
	"encoding/json"
	"testing"
	"time"
)

func validEnvelope() Envelope {
	return Envelope{
		ID:         "7e1b2f4b-7e1b-4f4b-9e1b-2f4b7e1b4f4b",
		Type:       "notification.requested.v1",
		Version:    1,
		OccurredAt: time.Now().UTC(),
		Producer:   "parcel",
		Data:       json.RawMessage(`{"recipient":"customer@example.com"}`),
	}
}

func TestEnvelopeValidate(t *testing.T) {
	if err := validEnvelope().Validate(); err != nil {
		t.Fatalf("valid envelope rejected: %v", err)
	}

	tests := map[string]func(*Envelope){
		"missing event id":     func(e *Envelope) { e.ID = "" },
		"missing event type":   func(e *Envelope) { e.Type = "" },
		"invalid version":      func(e *Envelope) { e.Version = 0 },
		"missing occurred at":  func(e *Envelope) { e.OccurredAt = time.Time{} },
		"missing producer":     func(e *Envelope) { e.Producer = "" },
		"missing data":         func(e *Envelope) { e.Data = nil },
		"invalid data payload": func(e *Envelope) { e.Data = json.RawMessage(`{`) },
	}

	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			envelope := validEnvelope()
			mutate(&envelope)
			if err := envelope.Validate(); err == nil {
				t.Fatal("Validate() should have failed")
			}
		})
	}
}
