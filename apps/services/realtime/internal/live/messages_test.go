package live

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

func decode(t *testing.T, data []byte) map[string]any {
	t.Helper()
	var value map[string]any
	if err := json.Unmarshal(data, &value); err != nil {
		t.Fatalf("not valid JSON: %v: %s", err, data)
	}
	return value
}

func TestUpdateMessageHasOnlyPublicFields(t *testing.T) {
	update := Update{
		TrackingNumber: trackingNumber,
		PreviousStatus: "IN_TRANSIT",
		Status:         "OUT_FOR_DELIVERY",
		UpdatedAt:      time.Date(2026, 11, 2, 21, 10, 0, 0, time.FixedZone("x", 5*3600+1800)),
	}

	got := decode(t, update.Message())
	want := map[string]any{
		"type": "parcel.status.updated",
		"data": map[string]any{
			"trackingNumber": trackingNumber,
			"previousStatus": "IN_TRANSIT",
			"status":         "OUT_FOR_DELIVERY",
			"updatedAt":      "2026-11-02T15:40:00Z",
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Message() = %v, want %v", got, want)
	}
}

func TestSubscribedMessage(t *testing.T) {
	one := decode(t, SubscribedMessage(ClientMessage{Action: ActionSubscribe, TrackingNumber: trackingNumber}))
	if !reflect.DeepEqual(one, map[string]any{"type": "subscribed", "trackingNumber": trackingNumber}) {
		t.Fatalf("subscribed (one) = %v", one)
	}
	all := decode(t, SubscribedMessage(ClientMessage{Action: ActionSubscribe, All: true}))
	if !reflect.DeepEqual(all, map[string]any{"type": "subscribed", "scope": "all"}) {
		t.Fatalf("subscribed (all) = %v", all)
	}
}

func TestErrorMessage(t *testing.T) {
	got := decode(t, ErrorMessage("FORBIDDEN", "Not allowed to subscribe to this scope."))
	want := map[string]any{"type": "error", "code": "FORBIDDEN", "message": "Not allowed to subscribe to this scope."}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ErrorMessage() = %v, want %v", got, want)
	}
}
