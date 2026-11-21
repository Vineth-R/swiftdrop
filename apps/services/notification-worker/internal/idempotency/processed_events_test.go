package idempotency

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestPostgresDeleteProcessedEventsBeforeUsesStrictCutoff(t *testing.T) {
	cutoff := time.Date(2026, time.November, 2, 12, 0, 0, 0, time.FixedZone("test", 5*60*60))
	db := &fakeDB{execTag: pgconn.NewCommandTag("DELETE 3")}
	store := NewPostgres(db, ConsumerName)

	deleted, err := store.DeleteProcessedEventsBefore(context.Background(), cutoff)
	if err != nil {
		t.Fatalf("DeleteProcessedEventsBefore() returned error: %v", err)
	}
	if deleted != 3 {
		t.Fatalf("deleted = %d, want 3", deleted)
	}
	if db.execCalls != 1 {
		t.Fatalf("exec calls = %d, want 1", db.execCalls)
	}
	if !strings.Contains(db.lastSQL, "DELETE FROM notification.processed_events") {
		t.Fatalf("query did not target processed_events: %s", db.lastSQL)
	}
	if !strings.Contains(db.lastSQL, "WHERE processed_at < $1") {
		t.Fatalf("query is not a strict cutoff delete: %s", db.lastSQL)
	}
	if len(db.lastArgs) != 1 {
		t.Fatalf("args = %v, want one cutoff", db.lastArgs)
	}
	gotCutoff, ok := db.lastArgs[0].(time.Time)
	if !ok {
		t.Fatalf("cutoff arg type = %T, want time.Time", db.lastArgs[0])
	}
	if !gotCutoff.Equal(cutoff) {
		t.Fatalf("cutoff = %v, want %v", gotCutoff, cutoff)
	}
	if gotCutoff.Location() != time.UTC {
		t.Fatalf("cutoff location = %v, want UTC", gotCutoff.Location())
	}
}

func TestPostgresDeleteProcessedEventsBeforeReturnsZero(t *testing.T) {
	db := &fakeDB{execTag: pgconn.NewCommandTag("DELETE 0")}
	store := NewPostgres(db, ConsumerName)

	deleted, err := store.DeleteProcessedEventsBefore(context.Background(), time.Now())
	if err != nil {
		t.Fatalf("DeleteProcessedEventsBefore() returned error: %v", err)
	}
	if deleted != 0 {
		t.Fatalf("deleted = %d, want 0", deleted)
	}
}

func TestPostgresDeleteProcessedEventsBeforeWrapsError(t *testing.T) {
	store := NewPostgres(&fakeDB{execErr: errors.New("database unavailable")}, ConsumerName)

	_, err := store.DeleteProcessedEventsBefore(context.Background(), time.Now())
	if err == nil || !errors.Is(err, ErrCleanupFailed) {
		t.Fatalf("error = %v, want ErrCleanupFailed", err)
	}
	if strings.Contains(err.Error(), "database unavailable") {
		t.Fatalf("error leaked driver text: %v", err)
	}
}

func TestPostgresDeleteProcessedEventsBeforeRespectsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	store := NewPostgres(&fakeDB{}, ConsumerName)

	_, err := store.DeleteProcessedEventsBefore(ctx, time.Now())
	if err == nil || !errors.Is(err, ErrCleanupFailed) || !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want ErrCleanupFailed and context.Canceled", err)
	}
}

func TestPostgresDeleteProcessedEventsBeforeWithoutDatabase(t *testing.T) {
	var store *Postgres
	if _, err := store.DeleteProcessedEventsBefore(context.Background(), time.Now()); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("nil store error = %v, want ErrUnavailable", err)
	}

	store = NewPostgres(nil, ConsumerName)
	if _, err := store.DeleteProcessedEventsBefore(context.Background(), time.Now()); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("nil database error = %v, want ErrUnavailable", err)
	}
}
