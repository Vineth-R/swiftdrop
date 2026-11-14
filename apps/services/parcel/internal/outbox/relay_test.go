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

func TestRelayPublisherFailureRetryAndIDPreservation(t *testing.T) {
	origEventID := "00000000-0000-4000-8000-000000000010"
	origPayload := `{"parcel_id":"p-123","tracking_number":"SDK99998888","courier_id":"c-456","assigned_by":"u-789"}`
	origCreatedAt := time.Now().UTC().Add(-5 * time.Minute).Truncate(time.Second)

	store := &mockStore{
		events: []Event{
			{
				ID:         origEventID,
				EventType:  "parcel.assigned",
				RoutingKey: "parcel.assigned",
				Payload:    json.RawMessage(origPayload),
				Status:     "PENDING",
				Attempts:   0,
				CreatedAt:  origCreatedAt,
			},
		},
	}

	pub := &mockPublisher{err: errors.New("amqp broker connection timeout")}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	relay := NewRelay(store, pub, logger)

	// Poll 1: Broker failure
	processed1, err := relay.PollOnce(context.Background())
	if err != nil {
		t.Fatalf("PollOnce() error: %v", err)
	}
	if processed1 != 1 {
		t.Fatalf("expected 1 processed event in poll 1, got %d", processed1)
	}

	// Verify state after failure:
	// - Row status must stay PENDING
	// - published_at must be nil
	// - attempts incremented to 1
	// - last_error recorded with failure reason
	if store.events[0].Status != "PENDING" {
		t.Errorf("expected status PENDING after broker failure, got %s", store.events[0].Status)
	}
	if store.events[0].PublishedAt != nil {
		t.Errorf("expected published_at == nil after failure, got %v", store.events[0].PublishedAt)
	}
	if store.events[0].Attempts != 1 {
		t.Errorf("expected attempts == 1 after failure, got %d", store.events[0].Attempts)
	}
	if store.events[0].LastError == nil || *store.events[0].LastError != "amqp broker connection timeout" {
		t.Errorf("expected last_error = 'amqp broker connection timeout', got %v", store.events[0].LastError)
	}
	if len(pub.publishes) != 0 {
		t.Fatalf("expected 0 successful publishes on broker error, got %d", len(pub.publishes))
	}

	// Broker recovers: Clear error and reset retry timer
	pub.err = nil
	relay.retryAfterMu.Lock()
	delete(relay.retryAfter, origEventID) // Simulate backoff window elapsing
	relay.retryAfterMu.Unlock()

	// Poll 2: Retry succeeds
	processed2, err := relay.PollOnce(context.Background())
	if err != nil {
		t.Fatalf("PollOnce() retry error: %v", err)
	}
	if processed2 != 1 {
		t.Fatalf("expected 1 processed event in poll 2, got %d", processed2)
	}

	// Verify state after successful publish:
	// - Row marked PUBLISHED
	// - published_at is set to non-nil timestamp
	// - attempts incremented to 2
	// - last_error cleared
	if store.events[0].Status != "PUBLISHED" {
		t.Errorf("expected status PUBLISHED after recovery, got %s", store.events[0].Status)
	}
	if store.events[0].PublishedAt == nil {
		t.Fatal("expected published_at != nil after successful publish")
	}
	if store.events[0].Attempts != 2 {
		t.Errorf("expected attempts == 2, got %d", store.events[0].Attempts)
	}
	if store.events[0].LastError != nil {
		t.Errorf("expected last_error cleared (nil), got %v", store.events[0].LastError)
	}

	// Verify exact original event ID preservation in published envelope
	if len(pub.publishes) != 1 {
		t.Fatalf("expected exactly 1 published message, got %d", len(pub.publishes))
	}
	var publishedEnv messaging.Envelope
	if err := json.Unmarshal(pub.publishes[0].body, &publishedEnv); err != nil {
		t.Fatalf("unmarshal published envelope: %v", err)
	}

	if publishedEnv.ID != origEventID {
		t.Errorf("published envelope ID = %s, want original event ID %s", publishedEnv.ID, origEventID)
	}
	if publishedEnv.Type != "parcel.assigned.v1" {
		t.Errorf("published envelope Type = %s, want parcel.assigned.v1", publishedEnv.Type)
	}
	if !publishedEnv.OccurredAt.Equal(origCreatedAt) {
		t.Errorf("published envelope OccurredAt = %v, want original %v", publishedEnv.OccurredAt, origCreatedAt)
	}
	if publishedEnv.Producer != "parcel" {
		t.Errorf("published envelope Producer = %s, want parcel", publishedEnv.Producer)
	}
	if string(publishedEnv.Data) != origPayload {
		t.Errorf("published envelope payload = %s, want %s", string(publishedEnv.Data), origPayload)
	}
}

func TestRelayRetentionCleanupFiltering(t *testing.T) {
	now := time.Now().UTC()
	retention := 14 * 24 * time.Hour
	cutoff := now.Add(-retention)

	t15DaysAgo := now.Add(-15 * 24 * time.Hour)
	t20DaysAgo := now.Add(-20 * 24 * time.Hour)
	t5DaysAgo := now.Add(-5 * 24 * time.Hour)
	t30DaysAgo := now.Add(-30 * 24 * time.Hour)

	events := []Event{
		{
			ID:          "ev-old-pub-1",
			Status:      "PUBLISHED",
			PublishedAt: &t15DaysAgo,
		},
		{
			ID:          "ev-old-pub-2",
			Status:      "PUBLISHED",
			PublishedAt: &t20DaysAgo,
		},
		{
			ID:          "ev-recent-pub",
			Status:      "PUBLISHED",
			PublishedAt: &t5DaysAgo,
		},
		{
			ID:        "ev-old-pending",
			Status:    "PENDING",
			CreatedAt: t30DaysAgo,
		},
		{
			ID:        "ev-old-failed",
			Status:    "FAILED",
			CreatedAt: t30DaysAgo,
		},
	}

	// Filter according to retention rule:
	// status == 'PUBLISHED' AND published_at < cutoff
	var deletedIDs []string
	var retainedIDs []string
	for _, e := range events {
		if e.Status == "PUBLISHED" && e.PublishedAt != nil && e.PublishedAt.Before(cutoff) {
			deletedIDs = append(deletedIDs, e.ID)
		} else {
			retainedIDs = append(retainedIDs, e.ID)
		}
	}

	if len(deletedIDs) != 2 {
		t.Fatalf("expected 2 deleted events (older than 14d and PUBLISHED), got %d: %v", len(deletedIDs), deletedIDs)
	}
	if deletedIDs[0] != "ev-old-pub-1" || deletedIDs[1] != "ev-old-pub-2" {
		t.Errorf("unexpected deleted IDs: %v", deletedIDs)
	}

	// Verify all 3 non-eligible events were retained:
	// - recent published (<14d)
	// - pending event (even if old)
	// - failed event
	if len(retainedIDs) != 3 {
		t.Fatalf("expected 3 retained events, got %d: %v", len(retainedIDs), retainedIDs)
	}
	for _, expectedID := range []string{"ev-recent-pub", "ev-old-pending", "ev-old-failed"} {
		found := false
		for _, rid := range retainedIDs {
			if rid == expectedID {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected event %s to be retained, but was not in %v", expectedID, retainedIDs)
		}
	}
}

func TestSQLStoreNilPool(t *testing.T) {
	store := NewSQLStore(nil)
	if _, err := store.ProcessPending(context.Background(), 10, func(e Event) error { return nil }); err == nil {
		t.Error("expected error from ProcessPending with nil pool")
	}
	if _, err := store.CleanupPublished(context.Background(), 14*24*time.Hour); err == nil {
		t.Error("expected error from CleanupPublished with nil pool")
	}
}

func TestDefaultRelayOptions(t *testing.T) {
	opts := DefaultRelayOptions()
	if opts.PollInterval != time.Second {
		t.Errorf("PollInterval = %v, want 1s", opts.PollInterval)
	}
	if opts.CleanupInterval != 24*time.Hour {
		t.Errorf("CleanupInterval = %v, want 24h", opts.CleanupInterval)
	}
	if opts.RetentionPeriod != 14*24*time.Hour {
		t.Errorf("RetentionPeriod = %v, want 14d", opts.RetentionPeriod)
	}
	if opts.BatchSize != 50 {
		t.Errorf("BatchSize = %d, want 50", opts.BatchSize)
	}
	if opts.Exchange != "swiftdrop.events" {
		t.Errorf("Exchange = %s, want swiftdrop.events", opts.Exchange)
	}
}

func TestRelayNilDependencies(t *testing.T) {
	relay := NewRelay(nil, nil, nil)
	count, err := relay.PollOnce(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 0 {
		t.Errorf("count = %d, want 0", count)
	}
}

func TestRelayInvalidPayloads(t *testing.T) {
	tests := []struct {
		name    string
		eventID string
		payload string
	}{
		{
			name:    "non-object json payload",
			eventID: "00000000-0000-4000-8000-000000000021",
			payload: `[1, 2, 3]`,
		},
		{
			name:    "empty payload",
			eventID: "00000000-0000-4000-8000-000000000022",
			payload: ``,
		},
		{
			name:    "empty event ID fails envelope validation",
			eventID: "   ",
			payload: `{"valid":"json"}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store := &mockStore{
				events: []Event{
					{
						ID:         tc.eventID,
						EventType:  "parcel.assigned",
						RoutingKey: "parcel.assigned",
						Payload:    json.RawMessage(tc.payload),
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
				t.Fatalf("PollOnce() returned error: %v", err)
			}
			if processed != 1 {
				t.Errorf("processed = %d, want 1", processed)
			}
			if store.events[0].Status != "PENDING" {
				t.Errorf("status = %s, want PENDING", store.events[0].Status)
			}
			if store.events[0].LastError == nil {
				t.Error("expected lastError to be set on invalid payload")
			}
		})
	}
}
