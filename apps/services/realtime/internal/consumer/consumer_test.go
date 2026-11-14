package consumer

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Vineth-R/swiftdrop/apps/services/realtime/internal/live"
	"github.com/Vineth-R/swiftdrop/packages/go-common/messaging"
)

type recorder struct {
	updates     []live.Update
	assignments []live.AssignmentUpdate
}

func (r *recorder) Broadcast(u live.Update) int {
	r.updates = append(r.updates, u)
	return 1
}

func (r *recorder) BroadcastAssignment(u live.AssignmentUpdate) int {
	r.assignments = append(r.assignments, u)
	return 1
}

func event(eventType, data string) []byte {
	return []byte(`{"event_id":"e1","event_type":"` + eventType + `","event_version":1,` +
		`"occurred_at":"2026-11-04T10:00:00Z","producer":"parcel","data":` + data + `}`)
}

func TestHandleBroadcastsOnlyPublicFields(t *testing.T) {
	rec := &recorder{}
	body := event(EventType, `{"tracking_number":"SDK7M2Q9XP4","previous_status":"CREATED",`+
		`"status":"PICKED_UP","merchant_id":"m1","customer_user_id":"u1","note":"secret"}`)
	if err := New(rec).Handle(context.Background(), messaging.Message{Body: body}); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if len(rec.updates) != 1 {
		t.Fatalf("broadcasts = %d, want 1", len(rec.updates))
	}
	u := rec.updates[0]
	if u.TrackingNumber != "SDK7M2Q9XP4" || u.PreviousStatus != "CREATED" || u.Status != "PICKED_UP" {
		t.Fatalf("unexpected update %+v", u)
	}
	out := string(u.Message())
	for _, leak := range []string{"merchant", "customer", "secret"} {
		if strings.Contains(out, leak) {
			t.Fatalf("message leaks %q: %s", leak, out)
		}
	}
}

func TestHandleRejectsBadEventsPermanently(t *testing.T) {
	cases := map[string][]byte{
		"not json":       []byte("nope"),
		"wrong type":     event("parcel.created.v1", `{}`),
		"bad tracking":   event(EventType, `{"tracking_number":"x","previous_status":"CREATED","status":"PICKED_UP"}`),
		"bad status":     event(EventType, `{"tracking_number":"SDK7M2Q9XP4","previous_status":"CREATED","status":"NOPE"}`),
		"missing before": event(EventType, `{"tracking_number":"SDK7M2Q9XP4","status":"PICKED_UP"}`),
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			rec := &recorder{}
			err := New(rec).Handle(context.Background(), messaging.Message{Body: body})
			if err == nil || !messaging.IsPermanent(err) {
				t.Fatalf("err = %v, want permanent error", err)
			}
			if len(rec.updates) != 0 {
				t.Fatal("bad event was broadcast")
			}
		})
	}
}

func TestHandleSuppressesDuplicateEventID(t *testing.T) {
	rec := &recorder{}
	handler := New(rec)
	body := event(EventType, `{"tracking_number":"SDK7M2Q9XP4","previous_status":"CREATED","status":"PICKED_UP"}`)
	for i := 0; i < 2; i++ {
		if err := handler.Handle(context.Background(), messaging.Message{Body: body}); err != nil {
			t.Fatalf("Handle %d: %v", i, err)
		}
	}
	if len(rec.updates) != 1 {
		t.Fatalf("broadcasts = %d, want 1", len(rec.updates))
	}
}

func TestDedupeIsBoundedAndExpires(t *testing.T) {
	now := time.Now()
	dedupe := &eventDedupe{capacity: 2, ttl: time.Minute, seen: map[string]time.Time{}}
	if !dedupe.claim("a", now) || !dedupe.claim("b", now) || !dedupe.claim("c", now) {
		t.Fatal("new IDs must be claimed")
	}
	if !dedupe.claim("a", now) {
		t.Fatal("evicted ID should be claimable again")
	}
	if dedupe.claim("c", now) {
		t.Fatal("recent ID must still be suppressed")
	}
	if !dedupe.claim("c", now.Add(2*time.Minute)) {
		t.Fatal("expired ID should be claimable again")
	}
}

func TestAssignmentHandleBroadcastsOnlyPublicFields(t *testing.T) {
	rec := &recorder{}
	body := event(AssignmentEventType, `{"parcel_id":"p1","tracking_number":"SDK7M2Q9XP4",`+
		`"courier_id":"c-secret","assigned_by":"d-secret","previous_courier_id":"old-secret"}`)
	if err := NewAssignment(rec).Handle(context.Background(), messaging.Message{Body: body}); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if len(rec.assignments) != 1 {
		t.Fatalf("broadcasts = %d, want 1", len(rec.assignments))
	}
	a := rec.assignments[0]
	if a.TrackingNumber != "SDK7M2Q9XP4" || a.UpdatedAt.IsZero() {
		t.Fatalf("unexpected assignment %+v", a)
	}
	out := string(a.Message())
	for _, leak := range []string{"c-secret", "d-secret", "old-secret", "courier_id", "assigned_by", "parcel_id", "previous_courier_id"} {
		if strings.Contains(out, leak) {
			t.Errorf("assignment message leaks %q: %s", leak, out)
		}
	}
	if !strings.Contains(out, `"type":"parcel.assigned.updated"`) {
		t.Errorf("assignment message type = %s", out)
	}
}

func TestAssignmentHandleDeduplicates(t *testing.T) {
	rec := &recorder{}
	handler := NewAssignment(rec)
	body := event(AssignmentEventType, `{"tracking_number":"SDK7M2Q9XP4"}`)
	for i := 0; i < 2; i++ {
		if err := handler.Handle(context.Background(), messaging.Message{Body: body}); err != nil {
			t.Fatalf("Handle: %v", err)
		}
	}
	if len(rec.assignments) != 1 {
		t.Fatalf("broadcasts after duplicate = %d, want 1", len(rec.assignments))
	}
}

func TestAssignmentHandleRejectsUnexpectedEvents(t *testing.T) {
	cases := map[string]struct {
		body []byte
	}{
		"status event": {body: event(EventType, `{"tracking_number":"SDK7M2Q9XP4","previous_status":"CREATED","status":"PICKED_UP"}`)},
		"malformed":    {body: []byte(`not-json`)},
		"bad tracking": {body: event(AssignmentEventType, `{"tracking_number":"nope"}`)},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			err := NewAssignment(&recorder{}).Handle(context.Background(), messaging.Message{Body: tc.body})
			if err == nil || !messaging.IsPermanent(err) {
				t.Fatalf("error = %v, want permanent", err)
			}
		})
	}
}
