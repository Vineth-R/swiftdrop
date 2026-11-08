package webhookdelivery

import (
	"encoding/json"
	"time"
)

// EventTypeStatusChanged is the public event type merchants receive.
const EventTypeStatusChanged = "parcel.status_changed"

// publicSchemaVersion is the version of the public merchant webhook body
// (API v1.1, section 10.1). It is deliberately separate from the internal
// event_version in the RabbitMQ envelope.
const publicSchemaVersion = 1

// Event is one webhook event in the public shape (API v1.1, section 10.1).
// It deliberately carries no names, addresses or phone numbers.
type Event struct {
	ID         string
	Type       string
	OccurredAt time.Time
	Data       EventData
}

// EventData is the "data" object of the webhook body. Pointer fields are
// written as JSON null when absent, because the contract shows explicit nulls
// (for example "note": null) and merchants may rely on the keys being present.
type EventData struct {
	TrackingNumber  string  `json:"trackingNumber"`
	ExternalOrderID *string `json:"externalOrderId"`
	PreviousStatus  *string `json:"previousStatus"`
	Status          string  `json:"status"`
	Note            *string `json:"note"`
	FailureReason   *string `json:"failureReason"`
}

// wireEvent fixes the JSON key names and order of the request body.
type wireEvent struct {
	EventID       string    `json:"eventId"`
	EventType     string    `json:"eventType"`
	SchemaVersion int       `json:"schemaVersion"`
	OccurredAt    string    `json:"occurredAt"`
	Data          EventData `json:"data"`
}

// Body returns the exact bytes sent as the request body. The signature is
// computed over these bytes, so they must be produced once and reused.
func (e Event) Body() ([]byte, error) {
	return json.Marshal(wireEvent{
		EventID:       e.ID,
		EventType:     e.Type,
		SchemaVersion: publicSchemaVersion,
		OccurredAt:    e.OccurredAt.UTC().Format("2006-01-02T15:04:05Z"),
		Data:          e.Data,
	})
}

// SampleEvent builds the event sent by "Send test event" (API v1.1, 8.14). It
// uses obviously fake values so a merchant can tell it from a real delivery.
func SampleEvent(eventID string, now time.Time) Event {
	prev, order := "PICKED_UP", "TEST-ORDER-0001"
	return Event{
		ID:         eventID,
		Type:       EventTypeStatusChanged,
		OccurredAt: now,
		Data: EventData{
			TrackingNumber:  "SDNTEST0001",
			ExternalOrderID: &order,
			PreviousStatus:  &prev,
			Status:          "IN_TRANSIT",
		},
	}
}
