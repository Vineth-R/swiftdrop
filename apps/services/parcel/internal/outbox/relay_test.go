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

		if m.events[i].Attempts >= MaxAttempts {
			m.events[i].Status = "FAILED"
			processed++
			continue
		}

		err := handler(m.events[i])
		if errors.Is(err, ErrSkip) {
			continue
		}

		if err == nil {
			m.events[i].Status = "PUBLISHED"
			now := time.Now().UTC()
			m.events[i].PublishedAt = &now
			m.events[i].Attempts++
			m.events[i].LastError = nil
		} else {
			m.events[i].Attempts++
			if m.events[i].Attempts >= MaxAttempts {
				m.events[i].Status = "FAILED"
			}
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

func TestBackoffDuration(t *testing.T) {
	if got := BackoffDuration(0); got != 0 {
		t.Errorf("BackoffDuration(0) = %v, want 0", got)
	}

	// Attempt 1: base = 1s, half = 500ms, range [500ms, 1s]
	d1 := BackoffDuration(1)
	if d1 < 500*time.Millisecond || d1 > time.Second {
		t.Errorf("BackoffDuration(1) = %v, want between 500ms and 1s", d1)
	}

	// Attempt 5: base = 16s, half = 8s, range [8s, 16s]
	d5 := BackoffDuration(5)
	if d5 < 8*time.Second || d5 > 16*time.Second {
		t.Errorf("BackoffDuration(5) = %v, want between 8s and 16s", d5)
	}

	// Attempt 6+: capped at 30s maxDelay, half = 15s, range [15s, 30s]
	d6 := BackoffDuration(10)
	if d6 < 15*time.Second || d6 > 30*time.Second {
		t.Errorf("BackoffDuration(10) = %v, want between 15s and 30s", d6)
	}
}

func TestRelayPollOnceBackoffSkipsUntilReady(t *testing.T) {
	store := &mockStore{
		events: []Event{
			{
				ID:         "00000000-0000-4000-8000-000000000003",
				EventType:  "parcel.created",
				RoutingKey: "parcel.created",
				Payload:    json.RawMessage(`{"parcel_id":"p1"}`),
				Status:     "PENDING",
				Attempts:   0,
				CreatedAt:  time.Now().UTC(),
			},
		},
	}
	pub := &mockPublisher{err: errors.New("amqp down")}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	relay := NewRelay(store, pub, logger)

	// First poll fails and sets backoff
	processed, err := relay.PollOnce(context.Background())
	if err != nil {
		t.Fatalf("PollOnce() error: %v", err)
	}
	if processed != 1 {
		t.Fatalf("expected 1 processed event, got %d", processed)
	}
	if store.events[0].Attempts != 1 {
		t.Fatalf("expected 1 attempt, got %d", store.events[0].Attempts)
	}

	// Second poll immediately should skip the event because it is in backoff
	processed2, err := relay.PollOnce(context.Background())
	if err != nil {
		t.Fatalf("PollOnce() error: %v", err)
	}
	if processed2 != 0 {
		t.Fatalf("expected 0 processed events (skipped in backoff), got %d", processed2)
	}
	if store.events[0].Attempts != 1 {
		t.Fatalf("expected attempts to remain 1 while in backoff, got %d", store.events[0].Attempts)
	}
}

func TestRelayMaxAttemptsTransitionsToFailed(t *testing.T) {
	store := &mockStore{
		events: []Event{
			{
				ID:         "00000000-0000-4000-8000-000000000004",
				EventType:  "parcel.created",
				RoutingKey: "parcel.created",
				Payload:    json.RawMessage(`{"parcel_id":"p1"}`),
				Status:     "PENDING",
				Attempts:   4, // Already failed 4 times
				CreatedAt:  time.Now().UTC(),
			},
		},
	}
	pub := &mockPublisher{err: errors.New("permanent failure")}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	relay := NewRelay(store, pub, logger)

	processed, err := relay.PollOnce(context.Background())
	if err != nil {
		t.Fatalf("PollOnce() error: %v", err)
	}
	if processed != 1 {
		t.Fatalf("expected 1 processed event, got %d", processed)
	}
	if store.events[0].Attempts != 5 {
		t.Fatalf("expected 5 attempts, got %d", store.events[0].Attempts)
	}
	if store.events[0].Status != "FAILED" {
		t.Fatalf("expected status FAILED after 5 attempts, got %s", store.events[0].Status)
	}
	if store.events[0].LastError == nil || *store.events[0].LastError != "permanent failure" {
		t.Fatalf("expected lastError = permanent failure, got %v", store.events[0].LastError)
	}
}
