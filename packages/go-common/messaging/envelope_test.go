package messaging

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func validEnvelope() Envelope {
	return Envelope{
		ID:         "7e1b2f4b-7e1b-4f4b-9e1b-2f4b7e1b4f4b",
		Type:       "notification.requested.v1",
		Version:    1,
		OccurredAt: time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC),
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
		"whitespace event id":  func(e *Envelope) { e.ID = "   " },
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

func TestEnvelopeValidateEventTypeVersion(t *testing.T) {
	tests := map[string]struct {
		eventType string
		version   int
		valid     bool
	}{
		"matching single digit": {eventType: "notification.requested.v1", version: 1, valid: true},
		"matching multi digit":  {eventType: "parcel.created.v12", version: 12, valid: true},
		"mismatched version":    {eventType: "notification.requested.v1", version: 2},
		"missing suffix":        {eventType: "notification.requested", version: 1},
		"empty suffix":          {eventType: "notification.requested.v", version: 1},
		"non numeric suffix":    {eventType: "notification.requested.vx", version: 1},
		"zero suffix":           {eventType: "notification.requested.v0", version: 1},
		"leading zero suffix":   {eventType: "notification.requested.v01", version: 1},
		"trailing dot":          {eventType: "notification.requested.", version: 1},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			envelope := validEnvelope()
			envelope.Type = test.eventType
			envelope.Version = test.version

			err := envelope.Validate()
			if test.valid && err != nil {
				t.Fatalf("valid event type rejected: %v", err)
			}
			if !test.valid && err == nil {
				t.Fatal("Validate() should have failed")
			}
		})
	}
}

func TestEnvelopeValidateDataMustBeObject(t *testing.T) {
	tests := map[string]json.RawMessage{
		"null":           json.RawMessage(`null`),
		"array":          json.RawMessage(`[{"recipient":"customer@example.com"}]`),
		"string":         json.RawMessage(`"recipient"`),
		"number":         json.RawMessage(`42`),
		"boolean":        json.RawMessage(`true`),
		"whitespace":     json.RawMessage(`   `),
		"trailing token": json.RawMessage(`{} {}`),
	}

	for name, data := range tests {
		t.Run(name, func(t *testing.T) {
			envelope := validEnvelope()
			envelope.Data = data
			if err := envelope.Validate(); err == nil {
				t.Fatal("Validate() should have failed")
			}
		})
	}
}

func TestEnvelopeValidateAcceptsUnknownDataFields(t *testing.T) {
	envelope := validEnvelope()
	envelope.Data = json.RawMessage(`{"recipient":"customer@example.com","future_field":{"nested":true}}`)

	if err := envelope.Validate(); err != nil {
		t.Fatalf("additive data field rejected: %v", err)
	}
}

func TestEnvelopeValidateDoesNotLeakData(t *testing.T) {
	envelope := validEnvelope()
	envelope.Data = json.RawMessage(`{"recipient":"secret@example.com"`)

	err := envelope.Validate()
	if err == nil {
		t.Fatal("Validate() should have failed")
	}
	if strings.Contains(err.Error(), "secret@example.com") {
		t.Fatalf("error leaked message data: %v", err)
	}
}
