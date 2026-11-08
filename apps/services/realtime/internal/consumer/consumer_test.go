package consumer

import (
	"context"
	"strings"
	"testing"

	"github.com/Vineth-R/swiftdrop/apps/services/realtime/internal/live"
	"github.com/Vineth-R/swiftdrop/packages/go-common/messaging"
)

type recorder struct{ updates []live.Update }

func (r *recorder) Broadcast(u live.Update) int {
	r.updates = append(r.updates, u)
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
