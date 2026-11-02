package idempotency

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const testEventID = "11111111-1111-4111-8111-111111111111"

type fakeRow struct {
	value bool
	err   error
}

func (r fakeRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	target, ok := dest[0].(*bool)
	if !ok {
		return errors.New("unexpected scan target")
	}
	*target = r.value
	return nil
}

type fakeDB struct {
	rowValue bool
	rowErr   error
	execErr  error

	queryCalls int
	execCalls  int
	lastSQL    string
	lastArgs   []any
}

func (d *fakeDB) QueryRow(_ context.Context, sql string, args ...any) pgx.Row {
	d.queryCalls++
	d.lastSQL = sql
	d.lastArgs = args
	return fakeRow{value: d.rowValue, err: d.rowErr}
}

func (d *fakeDB) Exec(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	d.execCalls++
	d.lastSQL = sql
	d.lastArgs = args
	return pgconn.CommandTag{}, d.execErr
}

func TestPostgresIsProcessed(t *testing.T) {
	tests := map[string]bool{
		"processed":     true,
		"not processed": false,
	}

	for name, want := range tests {
		t.Run(name, func(t *testing.T) {
			db := &fakeDB{rowValue: want}
			store := NewPostgres(db, ConsumerName)

			got, err := store.IsProcessed(context.Background(), testEventID)
			if err != nil {
				t.Fatalf("IsProcessed() returned error: %v", err)
			}
			if got != want {
				t.Fatalf("IsProcessed() = %v, want %v", got, want)
			}
			if db.queryCalls != 1 {
				t.Fatalf("query calls = %d, want 1", db.queryCalls)
			}
			if len(db.lastArgs) != 2 || db.lastArgs[0] != ConsumerName || db.lastArgs[1] != testEventID {
				t.Fatalf("query args = %v, want consumer and event ID", db.lastArgs)
			}
			if !strings.Contains(db.lastSQL, "notification.processed_events") {
				t.Fatalf("query did not target processed_events: %s", db.lastSQL)
			}
		})
	}
}

func TestPostgresIsProcessedWrapsError(t *testing.T) {
	store := NewPostgres(&fakeDB{rowErr: errors.New("database unavailable")}, ConsumerName)

	_, err := store.IsProcessed(context.Background(), testEventID)
	if err == nil {
		t.Fatal("IsProcessed() should have failed")
	}
	if !strings.Contains(err.Error(), "query processed event") {
		t.Fatalf("error = %q, want context", err)
	}
}

func TestPostgresMarkProcessedUsesConflictSafeInsert(t *testing.T) {
	db := &fakeDB{}
	store := NewPostgres(db, ConsumerName)

	if err := store.MarkProcessed(context.Background(), testEventID); err != nil {
		t.Fatalf("MarkProcessed() returned error: %v", err)
	}
	if db.execCalls != 1 {
		t.Fatalf("exec calls = %d, want 1", db.execCalls)
	}
	if len(db.lastArgs) != 2 || db.lastArgs[0] != ConsumerName || db.lastArgs[1] != testEventID {
		t.Fatalf("exec args = %v, want consumer and event ID", db.lastArgs)
	}
	if !strings.Contains(db.lastSQL, "ON CONFLICT (consumer, event_id) DO NOTHING") {
		t.Fatalf("insert is not conflict-safe: %s", db.lastSQL)
	}
}

func TestPostgresMarkProcessedWrapsError(t *testing.T) {
	store := NewPostgres(&fakeDB{execErr: errors.New("database unavailable")}, ConsumerName)

	err := store.MarkProcessed(context.Background(), testEventID)
	if err == nil {
		t.Fatal("MarkProcessed() should have failed")
	}
	if !strings.Contains(err.Error(), "record processed event") {
		t.Fatalf("error = %q, want context", err)
	}
}

func TestPostgresWithoutDatabaseIsUnavailable(t *testing.T) {
	t.Run("nil store", func(t *testing.T) {
		var store *Postgres
		if _, err := store.IsProcessed(context.Background(), testEventID); !errors.Is(err, ErrUnavailable) {
			t.Fatalf("IsProcessed() error = %v, want ErrUnavailable", err)
		}
		if err := store.MarkProcessed(context.Background(), testEventID); !errors.Is(err, ErrUnavailable) {
			t.Fatalf("MarkProcessed() error = %v, want ErrUnavailable", err)
		}
	})

	t.Run("nil database", func(t *testing.T) {
		store := NewPostgres(nil, ConsumerName)
		if _, err := store.IsProcessed(context.Background(), testEventID); !errors.Is(err, ErrUnavailable) {
			t.Fatalf("IsProcessed() error = %v, want ErrUnavailable", err)
		}
		if err := store.MarkProcessed(context.Background(), testEventID); !errors.Is(err, ErrUnavailable) {
			t.Fatalf("MarkProcessed() error = %v, want ErrUnavailable", err)
		}
	})
}

func TestPostgresScopesByConsumer(t *testing.T) {
	db := &fakeDB{}
	store := NewPostgres(db, "custom-consumer")

	if _, err := store.IsProcessed(context.Background(), testEventID); err != nil {
		t.Fatalf("IsProcessed() returned error: %v", err)
	}
	if db.lastArgs[0] != "custom-consumer" {
		t.Fatalf("consumer = %v, want custom-consumer", db.lastArgs[0])
	}
}
