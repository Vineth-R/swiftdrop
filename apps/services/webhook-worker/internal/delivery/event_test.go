package delivery

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestEventBodyMatchesContract(t *testing.T) {
	prev, order := "PICKED_UP", "SZ-100245"
	ev := Event{
		ID:         "ev12a3b4-5c6d-4e7f-8a9b-0c1d2e3f4a5b",
		Type:       EventTypeStatusChanged,
		OccurredAt: time.Date(2026, 11, 4, 18, 30, 0, 0, time.FixedZone("x", 5*3600+1800)), // 13:00:00Z
		Data: EventData{
			TrackingNumber:  "SDN4T8W2LC6",
			ExternalOrderID: &order,
			PreviousStatus:  &prev,
			Status:          "IN_TRANSIT",
		},
	}
	got, err := ev.Body()
	if err != nil {
		t.Fatal(err)
	}
	// Same keys, order and explicit nulls as API v1.1 section 10.1.
	const want = `{"eventId":"ev12a3b4-5c6d-4e7f-8a9b-0c1d2e3f4a5b","eventType":"parcel.status_changed",` +
		`"schemaVersion":1,"occurredAt":"2026-11-04T13:00:00Z",` +
		`"data":{"trackingNumber":"SDN4T8W2LC6","externalOrderId":"SZ-100245",` +
		`"previousStatus":"PICKED_UP","status":"IN_TRANSIT","note":null,"failureReason":null}}`
	if string(got) != want {
		t.Fatalf("Body() =\n%s\nwant\n%s", got, want)
	}
}

// The public body must carry the documented schema version on every event.
func TestEventBodyIncludesSchemaVersion(t *testing.T) {
	for _, ev := range []Event{
		{
			ID:         "e1",
			Type:       EventTypeStatusChanged,
			OccurredAt: time.Unix(0, 0),
			Data:       EventData{TrackingNumber: "SDN1", Status: "CREATED"},
		},
		SampleEvent("e2", time.Unix(0, 0)),
	} {
		body, err := ev.Body()
		if err != nil {
			t.Fatal(err)
		}
		var decoded map[string]any
		if err := json.Unmarshal(body, &decoded); err != nil {
			t.Fatal(err)
		}
		got, present := decoded["schemaVersion"]
		if !present {
			t.Fatalf("body has no schemaVersion: %s", body)
		}
		if got != float64(publicSchemaVersion) {
			t.Fatalf("schemaVersion = %v, want %d", got, publicSchemaVersion)
		}
	}
}

func TestEventBodyKeepsNullsForMissingFields(t *testing.T) {
	ev := Event{ID: "e", Type: EventTypeStatusChanged, OccurredAt: time.Unix(0, 0), Data: EventData{TrackingNumber: "SDK1", Status: "CREATED"}}
	body, _ := ev.Body()
	var decoded map[string]any
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatal(err)
	}
	data := decoded["data"].(map[string]any)
	for _, key := range []string{"externalOrderId", "previousStatus", "note", "failureReason"} {
		v, present := data[key]
		if !present || v != nil {
			t.Errorf("data.%s = %v (present=%v); want an explicit null", key, v, present)
		}
	}
}

// The public body must never carry personal data (API v1.1, section 11.3).
func TestEventBodyHasNoPersonalFields(t *testing.T) {
	body, _ := SampleEvent("e", time.Unix(0, 0)).Body()
	for _, banned := range []string{"address", "phone", "recipientName", "senderName", "email", "token"} {
		if strings.Contains(strings.ToLower(string(body)), strings.ToLower(banned)) {
			t.Errorf("body contains %q: %s", banned, body)
		}
	}
}

func TestSampleEventIsMarkedAsTest(t *testing.T) {
	ev := SampleEvent("e1", time.Unix(0, 0))
	if ev.Type != EventTypeStatusChanged || ev.Data.TrackingNumber != "SDNTEST0001" {
		t.Fatalf("unexpected sample event: %+v", ev)
	}
}
