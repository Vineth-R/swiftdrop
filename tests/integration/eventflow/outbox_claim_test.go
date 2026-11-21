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

// TestOutboxRelayDoesNotPublishAfterLostClaim proves the per-event claim refresh
// closes the slow-publish window: when another replica has reclaimed a row that
// was still waiting in the batch, the original relay detects the changed token
// and skips the publish instead of duplicating it.
func TestOutboxRelayDoesNotPublishAfterLostClaim(t *testing.T) {
	pool, _ := newIsolatedDatabase(t)

	ids := insertPendingOutbox(t, pool, 2)
	store := parcelevents.NewSQLStore(pool)

	published := map[string]int{}
	stolen := ""
	handler := func(event parcelevents.Event) error {
		published[event.ID]++
		if stolen == "" {
			// Simulate another replica reclaiming the other row that is still
			// waiting its turn in this batch.
			stolen = ids[1]
			if event.ID == ids[1] {
				stolen = ids[0]
			}
			if _, err := pool.Exec(t.Context(), `UPDATE parcel.outbox_events
				SET claim_token = gen_random_uuid(), claimed_at = now()
				WHERE id = $1`, stolen); err != nil {
				return fmt.Errorf("simulate reclaim: %w", err)
			}
		}
		return nil
	}

	processed, err := store.ProcessPending(t.Context(), 2, handler)
	if err != nil {
		t.Fatalf("ProcessPending: %v", err)
	}
	if processed != 1 {
		t.Errorf("processed = %d, want 1 (the reclaimed row is not ours)", processed)
	}
	if stolen == "" {
		t.Fatal("the handler did not run")
	}
	if published[stolen] != 0 {
		t.Errorf("reclaimed event published %d times, want 0", published[stolen])
	}
	if got := scalar(t, pool, "SELECT status FROM parcel.outbox_events WHERE id='"+stolen+"'"); got != "PENDING" {
		t.Errorf("reclaimed event status = %s, want PENDING", got)
	}
}

func TestFailedOutboxReplayAndExpiry(t *testing.T) {
	pool, _ := newIsolatedDatabase(t)
	store := parcelevents.NewSQLStore(pool)

	ids := insertPendingOutbox(t, pool, 3)
	old := time.Now().UTC().Add(-48 * time.Hour)
	for i, id := range ids {
		status, attempts := "FAILED", 5
		if i == 2 {
			status, attempts = "PENDING", 0
		}
		if _, err := pool.Exec(t.Context(), `
			UPDATE parcel.outbox_events
			SET status = $2, attempts = $3, created_at = $4
			WHERE id = $1`, id, status, attempts, old); err != nil {
			t.Fatalf("mark outbox event %s: %v", id, err)
		}
	}

	summary, err := store.FailedSummary(t.Context())
	if err != nil {
		t.Fatalf("FailedSummary returned error: %v", err)
	}
	if summary.Count != 2 || summary.Oldest == nil {
		t.Fatalf("summary = %+v, want two failed events", summary)
	}

	replayed, err := store.ReplayFailed(t.Context(), time.Now().UTC(), 10)
	if err != nil {
		t.Fatalf("ReplayFailed returned error: %v", err)
	}
	if replayed != 2 {
		t.Fatalf("replayed = %d, want 2", replayed)
	}
	for _, id := range ids[:2] {
		var status string
		var attempts int
		if err := pool.QueryRow(t.Context(), `
			SELECT status, attempts FROM parcel.outbox_events WHERE id = $1`, id).
			Scan(&status, &attempts); err != nil {
			t.Fatalf("read replayed event: %v", err)
		}
		if status != "PENDING" || attempts != 0 {
			t.Fatalf("event %s = %s/%d, want PENDING/0", id, status, attempts)
		}
	}

	// Return one event to FAILED and expire it; the other replayed events stay.
	if _, err := pool.Exec(t.Context(), `
		UPDATE parcel.outbox_events SET status = 'FAILED', attempts = 5, created_at = $2
		WHERE id = $1`, ids[0], old); err != nil {
		t.Fatalf("mark event failed: %v", err)
	}
	deleted, err := store.DeleteFailed(t.Context(), time.Now().UTC(), 10)
	if err != nil {
		t.Fatalf("DeleteFailed returned error: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("deleted = %d, want 1", deleted)
	}
	var remaining int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM parcel.outbox_events WHERE id = $1`, ids[0]).Scan(&remaining); err != nil {
		t.Fatalf("count deleted event: %v", err)
	}
	if remaining != 0 {
		t.Fatal("expired event still exists")
	}
}

func TestFailedOutboxConcurrentReplayClaimsEachRowOnce(t *testing.T) {
	pool, _ := newIsolatedDatabase(t)
	store := parcelevents.NewSQLStore(pool)
	ids := insertPendingOutbox(t, pool, 20)
	old := time.Now().UTC().Add(-48 * time.Hour)
	for _, id := range ids {
		if _, err := pool.Exec(t.Context(), `
			UPDATE parcel.outbox_events
			SET status = 'FAILED', attempts = 5, created_at = $2
			WHERE id = $1`, id, old); err != nil {
			t.Fatalf("mark outbox event failed: %v", err)
		}
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	var total int64
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			n, err := store.ReplayFailed(t.Context(), time.Now().UTC(), 50)
			if err != nil {
				t.Errorf("ReplayFailed: %v", err)
				return
			}
			mu.Lock()
			total += n
			mu.Unlock()
		}()
	}
	wg.Wait()
	if total != int64(len(ids)) {
		t.Fatalf("replayed %d rows across four workers, want %d", total, len(ids))
	}
	var failed, pending int
	if err := pool.QueryRow(t.Context(), `
		SELECT count(*) FILTER (WHERE status = 'FAILED'), count(*) FILTER (WHERE status = 'PENDING')
		FROM parcel.outbox_events`).Scan(&failed, &pending); err != nil {
		t.Fatalf("count replayed rows: %v", err)
	}
	if failed != 0 || pending != len(ids) {
		t.Fatalf("failed=%d pending=%d, want 0/%d", failed, pending, len(ids))
	}
}
