package idempotency

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const (
	testEventID  = "11111111-1111-4111-8111-111111111111"
	testParcelID = "22222222-2222-4222-8222-222222222222"
)

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

type fakeTx struct {
	pgx.Tx

	execCalls    int
	logErr       error
	processedErr error
	commitErr    error

	commits   int
	rollbacks int
	sqls      []string
	args      [][]any
}

func (t *fakeTx) Exec(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	t.execCalls++
	t.sqls = append(t.sqls, sql)
	t.args = append(t.args, args)
	if t.execCalls == 1 {
		return pgconn.CommandTag{}, t.logErr
	}
	return pgconn.CommandTag{}, t.processedErr
}

func (t *fakeTx) Commit(context.Context) error {
	t.commits++
	return t.commitErr
}

func (t *fakeTx) Rollback(context.Context) error {
	t.rollbacks++
	return nil
}

type fakeDB struct {
	rowValue bool
	rowErr   error
	execErr  error
	beginErr error

	queryCalls int
	execCalls  int
	lastSQL    string
	lastArgs   []any
	tx         *fakeTx
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

func (d *fakeDB) Begin(context.Context) (pgx.Tx, error) {
	if d.beginErr != nil {
		return nil, d.beginErr
	}
	if d.tx == nil {
		d.tx = &fakeTx{}
	}
	return d.tx, nil
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
	if err == nil || !errors.Is(err, ErrLookupFailed) {
		t.Fatalf("IsProcessed() error = %v, want ErrLookupFailed", err)
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
	if err == nil || !errors.Is(err, ErrMarkProcessedFailed) {
		t.Fatalf("MarkProcessed() error = %v, want ErrMarkProcessedFailed", err)
	}
}

func TestPostgresRecordDeliveredCommitsLogAndProcessedEvent(t *testing.T) {
	tx := &fakeTx{}
	db := &fakeDB{tx: tx}
	store := NewPostgres(db, ConsumerName)

	entry := LogEntry{
		EventID:   testEventID,
		ParcelID:  testParcelID,
		Channel:   "EMAIL",
		Recipient: "customer@example.com",
		Subject:   "Your parcel has shipped",
	}
	if err := store.RecordDelivered(context.Background(), entry); err != nil {
		t.Fatalf("RecordDelivered() returned error: %v", err)
	}

	if tx.execCalls != 2 {
		t.Fatalf("transaction exec calls = %d, want 2", tx.execCalls)
	}
	if tx.commits != 1 {
		t.Fatalf("commits = %d, want 1", tx.commits)
	}
	if !strings.Contains(tx.sqls[0], "notification.notification_logs") {
		t.Fatalf("first write did not target notification_logs: %s", tx.sqls[0])
	}
	if !strings.Contains(tx.sqls[0], "ON CONFLICT (event_id, channel, recipient)") {
		t.Fatalf("log upsert does not use the dedupe key: %s", tx.sqls[0])
	}
	if !strings.Contains(tx.sqls[0], "'SENT'") || !strings.Contains(tx.sqls[0], "now()") {
		t.Fatalf("delivered log does not use SENT and the database clock: %s", tx.sqls[0])
	}
	if len(tx.args[0]) != 5 {
		t.Fatalf("log args = %v, want 5 values", tx.args[0])
	}
	if !strings.Contains(tx.sqls[1], "notification.processed_events") {
		t.Fatalf("second write did not target processed_events: %s", tx.sqls[1])
	}
	if len(tx.args[1]) != 2 || tx.args[1][0] != ConsumerName || tx.args[1][1] != testEventID {
		t.Fatalf("processed args = %v, want consumer and event ID", tx.args[1])
	}
}

func TestPostgresRecordDeliveredRollsBackWhenLogWriteFails(t *testing.T) {
	tx := &fakeTx{logErr: errors.New("log write failed")}
	store := NewPostgres(&fakeDB{tx: tx}, ConsumerName)

	err := store.RecordDelivered(context.Background(), LogEntry{EventID: testEventID})
	if err == nil || !errors.Is(err, ErrRecordDeliveredFailed) {
		t.Fatalf("RecordDelivered() error = %v, want ErrRecordDeliveredFailed", err)
	}
	if !strings.Contains(err.Error(), "upsert log") {
		t.Fatalf("error = %q, want upsert-log context", err)
	}
	if tx.commits != 0 {
		t.Fatalf("commits = %d, want 0", tx.commits)
	}
	if tx.rollbacks == 0 {
		t.Fatal("transaction was not rolled back")
	}
}

func TestPostgresRecordDeliveredRollsBackWhenProcessedWriteFails(t *testing.T) {
	tx := &fakeTx{processedErr: errors.New("processed write failed")}
	store := NewPostgres(&fakeDB{tx: tx}, ConsumerName)

	err := store.RecordDelivered(context.Background(), LogEntry{EventID: testEventID})
	if err == nil || !errors.Is(err, ErrRecordDeliveredFailed) {
		t.Fatalf("RecordDelivered() error = %v, want ErrRecordDeliveredFailed", err)
	}
	if !strings.Contains(err.Error(), "insert processed event") {
		t.Fatalf("error = %q, want processed-event context", err)
	}
	if tx.commits != 0 {
		t.Fatalf("commits = %d, want 0", tx.commits)
	}
	if tx.rollbacks == 0 {
		t.Fatal("transaction was not rolled back")
	}
}

func TestPostgresRecordDeliveredReturnsBeginAndCommitErrors(t *testing.T) {
	t.Run("begin", func(t *testing.T) {
		store := NewPostgres(&fakeDB{beginErr: errors.New("no connection")}, ConsumerName)
		err := store.RecordDelivered(context.Background(), LogEntry{EventID: testEventID})
		if err == nil || !errors.Is(err, ErrRecordDeliveredFailed) {
			t.Fatalf("RecordDelivered() error = %v, want ErrRecordDeliveredFailed", err)
		}
		if !strings.Contains(err.Error(), "begin transaction") {
			t.Fatalf("error = %q, want begin context", err)
		}
	})

	t.Run("commit", func(t *testing.T) {
		tx := &fakeTx{commitErr: errors.New("commit failed")}
		store := NewPostgres(&fakeDB{tx: tx}, ConsumerName)
		err := store.RecordDelivered(context.Background(), LogEntry{EventID: testEventID})
		if err == nil || !errors.Is(err, ErrRecordDeliveredFailed) {
			t.Fatalf("RecordDelivered() error = %v, want ErrRecordDeliveredFailed", err)
		}
		if !strings.Contains(err.Error(), "commit") {
			t.Fatalf("error = %q, want commit context", err)
		}
	})
}

func TestPostgresRecordFailureUsesOutcomeStatus(t *testing.T) {
	db := &fakeDB{}
	store := NewPostgres(db, ConsumerName)

	entry := LogEntry{
		EventID:      testEventID,
		ParcelID:     testParcelID,
		Channel:      "EMAIL",
		Recipient:    "customer@example.com",
		Subject:      "Your parcel has shipped",
		Status:       StatusPending,
		ErrorMessage: "smtp connection refused",
	}
	if err := store.RecordFailure(context.Background(), entry); err != nil {
		t.Fatalf("RecordFailure() returned error: %v", err)
	}

	if db.execCalls != 1 {
		t.Fatalf("exec calls = %d, want 1", db.execCalls)
	}
	if !strings.Contains(db.lastSQL, "notification.notification_logs") {
		t.Fatalf("failure write did not target notification_logs: %s", db.lastSQL)
	}
	if !strings.Contains(db.lastSQL, "COALESCE(notification_logs.sent_at, EXCLUDED.sent_at)") {
		t.Fatalf("failure upsert does not preserve sent_at: %s", db.lastSQL)
	}
	if len(db.lastArgs) != 7 || db.lastArgs[5] != StatusPending || db.lastArgs[6] != "smtp connection refused" {
		t.Fatalf("failure args = %v, want PENDING with the error", db.lastArgs)
	}
}

func TestPostgresRecordFailureRejectsInvalidStatus(t *testing.T) {
	store := NewPostgres(&fakeDB{}, ConsumerName)

	err := store.RecordFailure(context.Background(), LogEntry{EventID: testEventID, Status: StatusSent})
	if err == nil || !errors.Is(err, ErrRecordFailureFailed) {
		t.Fatalf("RecordFailure() error = %v, want ErrRecordFailureFailed", err)
	}
}

func TestPostgresRecordFailureWrapsError(t *testing.T) {
	store := NewPostgres(&fakeDB{execErr: errors.New("database unavailable")}, ConsumerName)

	err := store.RecordFailure(context.Background(), LogEntry{EventID: testEventID, Status: StatusFailed})
	if err == nil || !errors.Is(err, ErrRecordFailureFailed) {
		t.Fatalf("RecordFailure() error = %v, want ErrRecordFailureFailed", err)
	}
}

// TestPostgresErrorsDoNotLeakDriverText proves that a driver message containing
// row values (including a recipient) never appears in a returned error.
func TestPostgresErrorsDoNotLeakDriverText(t *testing.T) {
	const secret = "customer@example.com"
	driverErr := errors.New("failing row contains (1111, 2222, EMAIL, " + secret + ", SD-SECRET)")

	tests := map[string]struct {
		store *Postgres
		call  func(*Postgres) error
	}{
		"lookup": {
			store: NewPostgres(&fakeDB{rowErr: driverErr}, ConsumerName),
			call: func(store *Postgres) error {
				_, err := store.IsProcessed(context.Background(), testEventID)
				return err
			},
		},
		"mark processed": {
			store: NewPostgres(&fakeDB{execErr: driverErr}, ConsumerName),
			call:  func(store *Postgres) error { return store.MarkProcessed(context.Background(), testEventID) },
		},
		"delivered begin": {
			store: NewPostgres(&fakeDB{beginErr: driverErr}, ConsumerName),
			call: func(store *Postgres) error {
				return store.RecordDelivered(context.Background(), LogEntry{EventID: testEventID})
			},
		},
		"delivered log": {
			store: NewPostgres(&fakeDB{tx: &fakeTx{logErr: driverErr}}, ConsumerName),
			call: func(store *Postgres) error {
				return store.RecordDelivered(context.Background(), LogEntry{EventID: testEventID})
			},
		},
		"delivered processed": {
			store: NewPostgres(&fakeDB{tx: &fakeTx{processedErr: driverErr}}, ConsumerName),
			call: func(store *Postgres) error {
				return store.RecordDelivered(context.Background(), LogEntry{EventID: testEventID})
			},
		},
		"delivered commit": {
			store: NewPostgres(&fakeDB{tx: &fakeTx{commitErr: driverErr}}, ConsumerName),
			call: func(store *Postgres) error {
				return store.RecordDelivered(context.Background(), LogEntry{EventID: testEventID})
			},
		},
		"failure": {
			store: NewPostgres(&fakeDB{execErr: driverErr}, ConsumerName),
			call: func(store *Postgres) error {
				return store.RecordFailure(context.Background(), LogEntry{EventID: testEventID, Status: StatusPending})
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			err := test.call(test.store)
			if err == nil {
				t.Fatal("expected an error")
			}
			if strings.Contains(err.Error(), secret) || strings.Contains(err.Error(), "SD-SECRET") {
				t.Fatalf("returned error leaked driver text: %v", err)
			}
		})
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
		if err := store.RecordDelivered(context.Background(), LogEntry{EventID: testEventID}); !errors.Is(err, ErrUnavailable) {
			t.Fatalf("RecordDelivered() error = %v, want ErrUnavailable", err)
		}
		if err := store.RecordFailure(context.Background(), LogEntry{EventID: testEventID, Status: StatusFailed}); !errors.Is(err, ErrUnavailable) {
			t.Fatalf("RecordFailure() error = %v, want ErrUnavailable", err)
		}
	})

	t.Run("nil database", func(t *testing.T) {
		store := NewPostgres(nil, ConsumerName)
		if _, err := store.IsProcessed(context.Background(), testEventID); !errors.Is(err, ErrUnavailable) {
			t.Fatalf("IsProcessed() error = %v, want ErrUnavailable", err)
		}
		if err := store.RecordDelivered(context.Background(), LogEntry{EventID: testEventID}); !errors.Is(err, ErrUnavailable) {
			t.Fatalf("RecordDelivered() error = %v, want ErrUnavailable", err)
		}
		if err := store.RecordFailure(context.Background(), LogEntry{EventID: testEventID, Status: StatusFailed}); !errors.Is(err, ErrUnavailable) {
			t.Fatalf("RecordFailure() error = %v, want ErrUnavailable", err)
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
