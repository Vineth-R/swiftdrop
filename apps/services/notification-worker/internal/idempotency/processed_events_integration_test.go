//go:build integration

package idempotency

import (
	"context"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Vineth-R/swiftdrop/packages/go-common/postgres"
)

const processedEventsTestTimeout = 120 * time.Second

// newProcessedEventsTestDatabase creates a uniquely named disposable database,
// applies the real notification migrations, and drops only that database.
func newProcessedEventsTestDatabase(t *testing.T) (context.Context, *postgres.Pool) {
	t.Helper()

	serverURL := os.Getenv("TEST_DATABASE_URL")
	if serverURL == "" {
		t.Skip("TEST_DATABASE_URL is not set; skipping PostgreSQL processed-event cleanup tests")
	}

	ctx, cancel := context.WithTimeout(context.Background(), processedEventsTestTimeout)
	t.Cleanup(cancel)

	admin, err := pgx.Connect(ctx, serverURL)
	if err != nil {
		t.Fatalf("connect to test database server: %v", err)
	}

	databaseName := "swiftdrop_processed_events_test_" + randomHex(8)
	quoted := pgx.Identifier{databaseName}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+quoted); err != nil {
		_ = admin.Close(ctx)
		t.Fatalf("create isolated database: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		if _, err := admin.Exec(cleanupCtx, "DROP DATABASE IF EXISTS "+quoted+" WITH (FORCE)"); err != nil {
			t.Logf("drop isolated database %s: %v", databaseName, err)
		}
		_ = admin.Close(cleanupCtx)
	})

	parsed, err := url.Parse(serverURL)
	if err != nil {
		t.Fatalf("parse TEST_DATABASE_URL: %v", err)
	}
	parsed.Path = "/" + databaseName

	config := postgres.DefaultConfig()
	config.URL = parsed.String()
	config.MaxConns = 8
	config.MinConns = 0
	config.RetryAttempts = 1
	pool, err := postgres.Open(ctx, config)
	if err != nil {
		t.Fatalf("open isolated database pool: %v", err)
	}
	t.Cleanup(pool.Close)

	root := filepath.Join("..", "..", "..", "..", "..")
	for _, file := range []string{
		"infra/docker/postgres/init/01-schemas.sql",
		"migrations/notification/000001_initial_schema.up.sql",
		"migrations/notification/000002_tables.up.sql",
	} {
		script, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(file)))
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		if _, err := pool.Exec(ctx, string(script)); err != nil {
			t.Fatalf("apply %s: %v", file, err)
		}
	}
	return ctx, pool
}

func insertProcessedEvent(
	t *testing.T,
	ctx context.Context,
	pool *postgres.Pool,
	consumer, eventID string,
	processedAt time.Time,
) {
	t.Helper()
	if _, err := pool.Exec(ctx, `
		INSERT INTO notification.processed_events (consumer, event_id, processed_at)
		VALUES ($1, $2::uuid, $3)`,
		consumer, eventID, processedAt); err != nil {
		t.Fatalf("insert processed event: %v", err)
	}
}

func insertDeliveryLog(
	t *testing.T,
	ctx context.Context,
	pool *postgres.Pool,
	eventID, status string,
	createdAt time.Time,
) {
	t.Helper()
	var errorMessage *string
	if status == StatusFailed {
		message := "delivery failed"
		errorMessage = &message
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO notification.notification_logs
			(event_id, parcel_id, channel, recipient, subject, status, error_message, created_at)
		VALUES ($1::uuid, $2::uuid, 'EMAIL', $3, 'Cleanup test', $4, $5, $6)`,
		eventID, randomUUID(t), "cleanup-"+eventID+"@example.com", status, errorMessage, createdAt); err != nil {
		t.Fatalf("insert notification log: %v", err)
	}
}

func countProcessedEvents(t *testing.T, ctx context.Context, pool *postgres.Pool) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM notification.processed_events`).Scan(&count); err != nil {
		t.Fatalf("count processed events: %v", err)
	}
	return count
}

func readDeliveryLogStatuses(t *testing.T, ctx context.Context, pool *postgres.Pool) []string {
	t.Helper()
	rows, err := pool.Query(ctx, `SELECT status FROM notification.notification_logs ORDER BY status`)
	if err != nil {
		t.Fatalf("query notification logs: %v", err)
	}
	defer rows.Close()

	statuses := []string{}
	for rows.Next() {
		var status string
		if err := rows.Scan(&status); err != nil {
			t.Fatalf("scan notification log status: %v", err)
		}
		statuses = append(statuses, status)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate notification logs: %v", err)
	}
	return statuses
}

func TestDeleteProcessedEventsBeforeKeepsCutoffAndNewer(t *testing.T) {
	ctx, pool := newProcessedEventsTestDatabase(t)
	store := NewPostgres(pool, ConsumerName)
	cutoff := time.Date(2026, time.November, 2, 12, 0, 0, 0, time.UTC)

	oldID := randomUUID(t)
	exactID := randomUUID(t)
	newID := randomUUID(t)
	insertProcessedEvent(t, ctx, pool, ConsumerName, oldID, cutoff.Add(-time.Minute))
	insertProcessedEvent(t, ctx, pool, ConsumerName, exactID, cutoff)
	insertProcessedEvent(t, ctx, pool, ConsumerName, newID, cutoff.Add(time.Minute))

	for _, status := range []string{StatusSent, StatusPending, StatusFailed} {
		insertDeliveryLog(t, ctx, pool, randomUUID(t), status, cutoff.Add(-24*time.Hour))
	}

	deleted, err := store.DeleteProcessedEventsBefore(ctx, cutoff)
	if err != nil {
		t.Fatalf("DeleteProcessedEventsBefore() returned error: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("deleted = %d, want 1", deleted)
	}
	if count := countProcessedEvents(t, ctx, pool); count != 2 {
		t.Fatalf("processed events = %d, want 2", count)
	}

	oldProcessed, err := store.IsProcessed(ctx, oldID)
	if err != nil {
		t.Fatalf("IsProcessed(old) returned error: %v", err)
	}
	if oldProcessed {
		t.Fatal("old processed event was not deleted")
	}
	for name, eventID := range map[string]string{"cutoff": exactID, "newer": newID} {
		processed, err := store.IsProcessed(ctx, eventID)
		if err != nil {
			t.Fatalf("IsProcessed(%s) returned error: %v", name, err)
		}
		if !processed {
			t.Fatalf("%s processed event was deleted", name)
		}
	}

	statuses := readDeliveryLogStatuses(t, ctx, pool)
	sort.Strings(statuses)
	wantStatuses := []string{StatusFailed, StatusPending, StatusSent}
	if strings.Join(statuses, ",") != strings.Join(wantStatuses, ",") {
		t.Fatalf("delivery log statuses = %v, want %v", statuses, wantStatuses)
	}

	deleted, err = store.DeleteProcessedEventsBefore(ctx, cutoff)
	if err != nil {
		t.Fatalf("repeated DeleteProcessedEventsBefore() returned error: %v", err)
	}
	if deleted != 0 {
		t.Fatalf("repeated deleted = %d, want 0", deleted)
	}
	if count := countProcessedEvents(t, ctx, pool); count != 2 {
		t.Fatalf("processed events after repeat = %d, want 2", count)
	}
	if statuses := readDeliveryLogStatuses(t, ctx, pool); strings.Join(statuses, ",") != strings.Join(wantStatuses, ",") {
		t.Fatalf("delivery log statuses after repeat = %v, want %v", statuses, wantStatuses)
	}
}

func TestDeleteProcessedEventsBeforeEmptyTable(t *testing.T) {
	ctx, pool := newProcessedEventsTestDatabase(t)
	store := NewPostgres(pool, ConsumerName)

	deleted, err := store.DeleteProcessedEventsBefore(ctx, time.Now().UTC())
	if err != nil {
		t.Fatalf("DeleteProcessedEventsBefore() returned error: %v", err)
	}
	if deleted != 0 {
		t.Fatalf("deleted = %d, want 0", deleted)
	}
}

func TestDeleteProcessedEventsBeforeReturnsStableError(t *testing.T) {
	ctx, pool := newProcessedEventsTestDatabase(t)
	store := NewPostgres(pool, ConsumerName)
	if _, err := pool.Exec(ctx, `DROP TABLE notification.processed_events`); err != nil {
		t.Fatalf("drop processed events table: %v", err)
	}

	_, err := store.DeleteProcessedEventsBefore(ctx, time.Now().UTC())
	if err == nil || !errors.Is(err, ErrCleanupFailed) {
		t.Fatalf("error = %v, want ErrCleanupFailed", err)
	}
	if strings.Contains(err.Error(), "processed_events") {
		t.Fatalf("error leaked database details: %v", err)
	}
}

func TestDeleteProcessedEventsBeforeRespectsCancellation(t *testing.T) {
	_, pool := newProcessedEventsTestDatabase(t)
	store := NewPostgres(pool, ConsumerName)
	canceled, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := store.DeleteProcessedEventsBefore(canceled, time.Now().UTC())
	if err == nil || !errors.Is(err, ErrCleanupFailed) || !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want ErrCleanupFailed and context.Canceled", err)
	}
}

func TestDeleteProcessedEventsBeforeConcurrent(t *testing.T) {
	ctx, pool := newProcessedEventsTestDatabase(t)
	store := NewPostgres(pool, ConsumerName)
	cutoff := time.Date(2026, time.November, 2, 12, 0, 0, 0, time.UTC)

	const rows = 20
	for range rows {
		insertProcessedEvent(t, ctx, pool, ConsumerName, randomUUID(t), cutoff.Add(-time.Hour))
	}

	const workers = 2
	var wait sync.WaitGroup
	counts := make(chan int64, workers)
	errs := make(chan error, workers)
	for range workers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			deleted, err := store.DeleteProcessedEventsBefore(ctx, cutoff)
			counts <- deleted
			errs <- err
		}()
	}
	wait.Wait()
	close(counts)
	close(errs)

	var total int64
	for deleted := range counts {
		total += deleted
	}
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent cleanup returned error: %v", err)
		}
	}
	if total != rows {
		t.Fatalf("total deleted = %d, want %d", total, rows)
	}
	if count := countProcessedEvents(t, ctx, pool); count != 0 {
		t.Fatalf("processed events after concurrent cleanup = %d, want 0", count)
	}
}
