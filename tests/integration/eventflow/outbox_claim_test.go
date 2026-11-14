//go:build integration

package eventflow

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	parcelevents "github.com/Vineth-R/swiftdrop/apps/services/parcel/eventtest"
	"github.com/Vineth-R/swiftdrop/packages/go-common/postgres"
)

// collectingPublisher records each published event ID. It is safe for
// concurrent use because two relay replicas publish in their own goroutines.
type collectingPublisher struct {
	mu       sync.Mutex
	publish  map[string]int
	exchange string
}

func newCollectingPublisher() *collectingPublisher {
	return &collectingPublisher{publish: make(map[string]int)}
}

func (p *collectingPublisher) Publish(ctx context.Context, exchange, routingKey string, body []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	var envelope struct {
		ID string `json:"event_id"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return fmt.Errorf("decode envelope: %w", err)
	}
	p.mu.Lock()
	p.publish[envelope.ID]++
	p.mu.Unlock()
	// Widen the window in which the old unlocked SELECT could hand the same row
	// to both replicas.
	time.Sleep(2 * time.Millisecond)
	return nil
}

func (p *collectingPublisher) counts() map[string]int {
	p.mu.Lock()
	defer p.mu.Unlock()
	copied := make(map[string]int, len(p.publish))
	for id, n := range p.publish {
		copied[id] = n
	}
	return copied
}

// insertPendingOutbox inserts count PENDING status-change events and returns
// their IDs.
func insertPendingOutbox(t *testing.T, pool *postgres.Pool, count int) []string {
	t.Helper()
	ids := make([]string, 0, count)
	for i := 0; i < count; i++ {
		id := randomUUID()
		payload, err := json.Marshal(map[string]any{
			"parcel_id":       randomUUID(),
			"tracking_number": fmt.Sprintf("SDKC%08d", i),
			"previous_status": "CREATED",
			"status":          "PICKED_UP",
		})
		if err != nil {
			t.Fatalf("marshal outbox payload: %v", err)
		}
		_, err = pool.Exec(t.Context(), `INSERT INTO parcel.outbox_events (
			id, event_type, routing_key, payload, status, attempts, created_at
		) VALUES ($1, 'parcel.status_changed', 'parcel.status.changed', $2, 'PENDING', 0, now())`, id, payload)
		if err != nil {
			t.Fatalf("insert outbox event: %v", err)
		}
		ids = append(ids, id)
	}
	return ids
}

// TestOutboxRelayClaimsEachEventOnce proves two relay replicas sharing one
// database publish each pending event exactly once. Before the atomic claim
// fix, both replicas selected the same unlocked PENDING rows and published
// them twice.
func TestOutboxRelayClaimsEachEventOnce(t *testing.T) {
	pool, _ := newIsolatedDatabase(t)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	const total = 40
	ids := insertPendingOutbox(t, pool, total)

	newRelay := func(publisher parcelevents.Publisher) *parcelevents.Relay {
		options := parcelevents.DefaultRelayOptions()
		options.BatchSize = 10
		options.Exchange = "swiftdrop.events"
		return parcelevents.NewRelay(parcelevents.NewSQLStore(pool), publisher, logger, options)
	}

	publisherA := newCollectingPublisher()
	publisherB := newCollectingPublisher()
	relayA := newRelay(publisherA)
	relayB := newRelay(publisherB)

	start := make(chan struct{})
	var wg sync.WaitGroup
	run := func(publisher *collectingPublisher, relay *parcelevents.Relay) {
		defer wg.Done()
		<-start
		for i := 0; i < 40; i++ {
			count, err := relay.PollOnce(t.Context())
			if err != nil {
				t.Errorf("PollOnce: %v", err)
				return
			}
			if count == 0 {
				time.Sleep(time.Millisecond)
			}
		}
	}
	wg.Add(2)
	go run(publisherA, relayA)
	go run(publisherB, relayB)
	close(start)
	wg.Wait()

	counts := publisherA.counts()
	for id, n := range publisherB.counts() {
		counts[id] += n
	}

	if len(counts) != total {
		t.Fatalf("distinct published events = %d, want %d", len(counts), total)
	}
	for _, id := range ids {
		if counts[id] != 1 {
			t.Errorf("event %s published %d times, want exactly 1", id, counts[id])
		}
	}
	if got := scalar(t, pool, "SELECT count(*) FROM parcel.outbox_events WHERE status='PUBLISHED'"); got != fmt.Sprintf("%d", total) {
		t.Errorf("PUBLISHED rows = %s, want %d", got, total)
	}
	if got := scalar(t, pool, "SELECT count(*) FROM parcel.outbox_events WHERE claim_token IS NOT NULL"); got != "0" {
		t.Errorf("leftover claimed rows = %s, want 0", got)
	}
}

// TestOutboxRelayReclaimsStaleClaim proves a row whose claimant died during the
// broker publish is reclaimed after the stale window instead of being stuck.
func TestOutboxRelayReclaimsStaleClaim(t *testing.T) {
	pool, _ := newIsolatedDatabase(t)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	ids := insertPendingOutbox(t, pool, 1)
	if _, err := pool.Exec(t.Context(), `UPDATE parcel.outbox_events
		SET claim_token = gen_random_uuid(), claimed_at = now() - interval '5 minutes'
		WHERE id = $1`, ids[0]); err != nil {
		t.Fatalf("simulate stale claim: %v", err)
	}

	publisher := newCollectingPublisher()
	options := parcelevents.DefaultRelayOptions()
	options.BatchSize = 10
	relay := parcelevents.NewRelay(parcelevents.NewSQLStore(pool), publisher, logger, options)

	waitUntil(t, eventWaitTimeout, "stale claim reclaimed", func() error {
		if _, err := relay.PollOnce(t.Context()); err != nil {
			return err
		}
		if publisher.counts()[ids[0]] == 1 {
			return nil
		}
		return fmt.Errorf("stale claimed event not republished")
	})
	if got := scalar(t, pool, "SELECT status FROM parcel.outbox_events WHERE id='"+ids[0]+"'"); got != "PUBLISHED" {
		t.Errorf("status = %s, want PUBLISHED", got)
	}
}

// TestOutboxRelayDoesNotStealFreshClaim proves a claim held by a live replica is
// respected: a second relay cannot publish the same row while the claim is
// fresh.
func TestOutboxRelayDoesNotStealFreshClaim(t *testing.T) {
	pool, _ := newIsolatedDatabase(t)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	ids := insertPendingOutbox(t, pool, 1)
	if _, err := pool.Exec(t.Context(), `UPDATE parcel.outbox_events
		SET claim_token = gen_random_uuid(), claimed_at = now()
		WHERE id = $1`, ids[0]); err != nil {
		t.Fatalf("simulate fresh claim: %v", err)
	}

	publisher := newCollectingPublisher()
	options := parcelevents.DefaultRelayOptions()
	options.BatchSize = 10
	relay := parcelevents.NewRelay(parcelevents.NewSQLStore(pool), publisher, logger, options)

	processed, err := relay.PollOnce(t.Context())
	if err != nil {
		t.Fatalf("PollOnce: %v", err)
	}
	if processed != 0 {
		t.Errorf("processed = %d, want 0 for a fresh claim", processed)
	}
	if len(publisher.counts()) != 0 {
		t.Errorf("published %d events under a live claim, want 0", len(publisher.counts()))
	}
	if got := scalar(t, pool, "SELECT status FROM parcel.outbox_events WHERE id='"+ids[0]+"'"); got != "PENDING" {
		t.Errorf("status = %s, want PENDING", got)
	}
}
