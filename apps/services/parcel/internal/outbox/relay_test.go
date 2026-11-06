package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/Vineth-R/swiftdrop/packages/go-common/messaging"
)

type mockStore struct {
	mu        sync.Mutex
	events    []Event
	cleanupFn func(ctx context.Context, retention time.Duration) (int64, error)
}

func (m *mockStore) ProcessPending(ctx context.Context, limit int, handler func(event Event) error) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	processed := 0
	for i := range m.events {
		if m.events[i].Status != "PENDING" {
			continue
		}
		if processed >= limit {
			break
		}

		err := handler(m.events[i])
		if err == nil {
			m.events[i].Status = "PUBLISHED"
			now := time.Now().UTC()
			m.events[i].PublishedAt = &now
			m.events[i].Attempts++
			m.events[i].LastError = nil
		} else {
			m.events[i].Attempts++
			msg := err.Error()
			m.events[i].LastError = &msg
		}
		processed++
	}
	return processed, nil
}

func (m *mockStore) CleanupPublished(ctx context.Context, retention time.Duration) (int64, error) {
	if m.cleanupFn != nil {
		return m.cleanupFn(ctx, retention)
	}
	return 0, nil
}

type mockPublisher struct {
	mu        sync.Mutex
	publishes []struct {
		exchange   string
		routingKey string
		body       []byte
	}
	err error
}

func (m *mockPublisher) Publish(ctx context.Context, exchange, routingKey string, body []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return m.err
	}
	m.publishes = append(m.publishes, struct {
		exchange   string
		routingKey string
		body       []byte
	}{exchange: exchange, routingKey: routingKey, body: body})
	return nil
}

func TestRelayPollOnceSuccess(t *testing.T) {
	store := &mockStore{
		events: []Event{
			{
				ID:         "00000000-0000-4000-8000-000000000001",
				EventType:  "parcel.assigned",
				RoutingKey: "parcel.assigned",
				Payload:    json.RawMessage(`{"parcel_id":"p1","tracking_number":"SDK12345678","courier_id":"c1","assigned_by":"u1"}`),
				Status:     "PENDING",
				Attempts:   0,
				CreatedAt:  time.Now().UTC(),
			},
		},
	}
	pub := &mockPublisher{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	relay := NewRelay(store, pub, logger)

	processed, err := relay.PollOnce(context.Background())
	if err != nil {
		t.Fatalf("PollOnce() error: %v", err)
	}
	if processed != 1 {
		t.Fatalf("expected 1 processed event, got %d", processed)
	}

	if len(pub.publishes) != 1 {
		t.Fatalf("expected 1 publication, got %d", len(pub.publishes))
	}
	if pub.publishes[0].routingKey != "parcel.assigned.v1" {
		t.Errorf("routingKey = %s, want parcel.assigned.v1", pub.publishes[0].routingKey)
	}

	var env messaging.Envelope
	if err := json.Unmarshal(pub.publishes[0].body, &env); err != nil {
		t.Fatalf("unmarshal published envelope: %v", err)
	}
	if env.Type != "parcel.assigned.v1" {
		t.Errorf("env.Type = %s, want parcel.assigned.v1", env.Type)
	}
	if env.Version != 1 {
		t.Errorf("env.Version = %d, want 1", env.Version)
	}

	if store.events[0].Status != "PUBLISHED" {
		t.Errorf("status = %s, want PUBLISHED", store.events[0].Status)
	}
	if store.events[0].Attempts != 1 {
		t.Errorf("attempts = %d, want 1", store.events[0].Attempts)
	}
}

func TestRelayPollOncePublisherFailureLeavesPending(t *testing.T) {
	store := &mockStore{
		events: []Event{
			{
				ID:         "00000000-0000-4000-8000-000000000002",
				EventType:  "parcel.status_changed",
				RoutingKey: "parcel.status.changed",
				Payload:    json.RawMessage(`{"parcel_id":"p1","tracking_number":"SDK12345678","previous_status":"CREATED","status":"PICKED_UP"}`),
				Status:     "PENDING",
				Attempts:   0,
				CreatedAt:  time.Now().UTC(),
			},
		},
	}
	pub := &mockPublisher{err: errors.New("rabbitmq connection lost")}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	relay := NewRelay(store, pub, logger)

	processed, err := relay.PollOnce(context.Background())
	if err != nil {
		t.Fatalf("PollOnce() error: %v", err)
	}
	if processed != 1 {
		t.Fatalf("expected 1 processed event, got %d", processed)
	}

	// Status must stay PENDING
	if store.events[0].Status != "PENDING" {
		t.Errorf("status = %s, want PENDING", store.events[0].Status)
	}
	if store.events[0].Attempts != 1 {
		t.Errorf("attempts = %d, want 1", store.events[0].Attempts)
	}
	if store.events[0].LastError == nil || *store.events[0].LastError != "rabbitmq connection lost" {
		t.Errorf("lastError = %v, want 'rabbitmq connection lost'", store.events[0].LastError)
	}
}

func TestMapEventRouting(t *testing.T) {
	tests := []struct {
		eventType  string
		routingKey string
		wantType   string
		wantKey    string
	}{
		{"parcel.created", "parcel.created", "parcel.created.v1", "parcel.created.v1"},
		{"parcel.created.v1", "parcel.created.v1", "parcel.created.v1", "parcel.created.v1"},
		{"parcel.assigned", "parcel.assigned", "parcel.assigned.v1", "parcel.assigned.v1"},
		{"parcel.assigned.v1", "parcel.assigned.v1", "parcel.assigned.v1", "parcel.assigned.v1"},
		{"parcel.status_changed", "parcel.status.changed", "parcel.status_changed.v1", "parcel.status_changed.v1"},
		{"parcel.status_changed.v1", "parcel.status_changed.v1", "parcel.status_changed.v1", "parcel.status_changed.v1"},
	}

	for _, tc := range tests {
		gotType, gotKey := mapEventRouting(tc.eventType, tc.routingKey)
		if gotType != tc.wantType || gotKey != tc.wantKey {
			t.Errorf("mapEventRouting(%s, %s) = (%s, %s), want (%s, %s)",
				tc.eventType, tc.routingKey, gotType, gotKey, tc.wantType, tc.wantKey)
		}
	}
}

func TestRelayCleanup(t *testing.T) {
	var cleanupCalled bool
	store := &mockStore{
		cleanupFn: func(ctx context.Context, retention time.Duration) (int64, error) {
			cleanupCalled = true
			if retention != 14*24*time.Hour {
				t.Errorf("retention = %v, want 14 days", retention)
			}
			return 5, nil
		},
	}
	pub := &mockPublisher{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	relay := NewRelay(store, pub, logger)
	relay.Run(ctx)

	if !cleanupCalled {
		t.Error("expected cleanup to be called on startup")
	}
}
