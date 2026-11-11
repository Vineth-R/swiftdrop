package merchantsync

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/orders"
	"github.com/Vineth-R/swiftdrop/packages/go-common/messaging"
)

const (
	testEventID  = "77777777-7777-4777-8777-777777777777"
	testMerchant = "11111111-1111-4111-8111-111111111111"
	testParcel   = "55555555-5555-4555-8555-555555555555"
	testTracking = "SDN4T8W2LC6"
)

type fakeStore struct {
	events  []orders.StatusEvent
	applied bool
	err     error
}

func (s *fakeStore) ApplyStatusEvent(_ context.Context, event orders.StatusEvent) (bool, error) {
	s.events = append(s.events, event)
	return s.applied, s.err
}

func testHandler(store Store) *Handler {
	return New(store, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func testMessage(data map[string]any) messaging.Message {
	if data == nil {
		data = map[string]any{
			"parcel_id": testParcel, "tracking_number": testTracking,
			"previous_status": "CREATED", "status": "IN_TRANSIT", "merchant_id": testMerchant,
		}
	}
	raw, _ := json.Marshal(data)
	envelope := messaging.Envelope{
		ID: testEventID, Type: eventType, Version: 1, OccurredAt: time.Now().UTC(),
		Producer: "parcel", CorrelationID: "corr-1", Data: raw,
	}
	body, _ := json.Marshal(envelope)
	return messaging.Message{Body: body, ContentType: "application/json"}
}

func TestHandleAppliesMerchantStatus(t *testing.T) {
	store := &fakeStore{applied: true}
	if err := testHandler(store).Handle(context.Background(), testMessage(nil)); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if len(store.events) != 1 {
		t.Fatalf("events = %d, want 1", len(store.events))
	}
	event := store.events[0]
	if event.EventID != testEventID || event.MerchantID != testMerchant || event.ParcelID != testParcel ||
		event.TrackingNumber != testTracking || event.Status != "IN_TRANSIT" || event.PreviousStatus != "CREATED" {
		t.Fatalf("event = %+v", event)
	}
}

func TestHandleIgnoresDuplicateOrOlderEvents(t *testing.T) {
	store := &fakeStore{applied: false}
	if err := testHandler(store).Handle(context.Background(), testMessage(nil)); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if len(store.events) != 1 {
		t.Fatalf("events = %d", len(store.events))
	}
}

func TestHandleIgnoresCustomerParcels(t *testing.T) {
	store := &fakeStore{applied: true}
	data := map[string]any{
		"parcel_id": testParcel, "tracking_number": "SDK7M2Q9XP4",
		"previous_status": "CREATED", "status": "IN_TRANSIT",
	}
	if err := testHandler(store).Handle(context.Background(), testMessage(data)); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if len(store.events) != 0 {
		t.Fatalf("customer event reached the store")
	}
}

func TestHandleAcknowledgesUnknownMerchantOrder(t *testing.T) {
	store := &fakeStore{err: orders.ErrNotFound}
	if err := testHandler(store).Handle(context.Background(), testMessage(nil)); err != nil {
		t.Fatalf("Handle() error = %v, want a safe acknowledgement", err)
	}
}

func TestHandleDeadLettersTrackingMismatch(t *testing.T) {
	store := &fakeStore{err: orders.ErrTrackingMismatch}
	err := testHandler(store).Handle(context.Background(), testMessage(nil))
	if !messaging.IsPermanent(err) {
		t.Fatalf("Handle() error = %v, want permanent", err)
	}
}

func TestHandleKeepsDatabaseFailuresRetryable(t *testing.T) {
	store := &fakeStore{err: errors.New("database down")}
	err := testHandler(store).Handle(context.Background(), testMessage(nil))
	if err == nil || messaging.IsPermanent(err) {
		t.Fatalf("Handle() error = %v, want retryable", err)
	}
}

func TestHandleRejectsMalformedEvents(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"invalid json", `not-json`},
		{"unsupported type", `{"event_id":"` + testEventID + `","event_type":"parcel.created.v1","event_version":1,"occurred_at":"2026-11-05T12:10:00Z","producer":"parcel","data":{}}`},
		{"wrong producer", `{"event_id":"` + testEventID + `","event_type":"parcel.status_changed.v1","event_version":1,"occurred_at":"2026-11-05T12:10:00Z","producer":"merchant","data":{}}`},
		{"version mismatch", `{"event_id":"` + testEventID + `","event_type":"parcel.status_changed.v1","event_version":2,"occurred_at":"2026-11-05T12:10:00Z","producer":"parcel","data":{}}`},
		{"bad event id", `{"event_id":"nope","event_type":"parcel.status_changed.v1","event_version":1,"occurred_at":"2026-11-05T12:10:00Z","producer":"parcel","data":{"parcel_id":"` + testParcel + `","tracking_number":"` + testTracking + `","previous_status":"CREATED","status":"IN_TRANSIT","merchant_id":"` + testMerchant + `"}}`},
		{"bad parcel id", `{"event_id":"` + testEventID + `","event_type":"parcel.status_changed.v1","event_version":1,"occurred_at":"2026-11-05T12:10:00Z","producer":"parcel","data":{"parcel_id":"nope","tracking_number":"` + testTracking + `","previous_status":"CREATED","status":"IN_TRANSIT","merchant_id":"` + testMerchant + `"}}`},
		{"bad status", `{"event_id":"` + testEventID + `","event_type":"parcel.status_changed.v1","event_version":1,"occurred_at":"2026-11-05T12:10:00Z","producer":"parcel","data":{"parcel_id":"` + testParcel + `","tracking_number":"` + testTracking + `","previous_status":"CREATED","status":"LOST","merchant_id":"` + testMerchant + `"}}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := &fakeStore{applied: true}
			err := testHandler(store).Handle(context.Background(), messaging.Message{Body: []byte(test.body), ContentType: "application/json"})
			if !messaging.IsPermanent(err) {
				t.Fatalf("Handle() error = %v, want permanent", err)
			}
			if len(store.events) != 0 {
				t.Fatalf("malformed event reached the store")
			}
		})
	}
}

func TestHandleRejectsNonJSONContentType(t *testing.T) {
	err := testHandler(&fakeStore{}).Handle(context.Background(), messaging.Message{Body: []byte(`{}`), ContentType: "text/plain"})
	if !messaging.IsPermanent(err) {
		t.Fatalf("Handle() error = %v, want permanent", err)
	}
}
