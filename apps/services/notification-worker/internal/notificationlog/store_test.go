package notificationlog

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/Vineth-R/swiftdrop/apps/services/notification-worker/internal/idempotency"
)

type fakeRow struct {
	total int
	err   error
}

func (r fakeRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	target, ok := dest[0].(*int)
	if !ok {
		return errors.New("unexpected scan target")
	}
	*target = r.total
	return nil
}

type fakeDB struct {
	row      fakeRow
	queryErr error
	execTag  pgconn.CommandTag
	execErr  error
}

func (d fakeDB) QueryRow(context.Context, string, ...any) pgx.Row {
	return d.row
}

func (d fakeDB) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, d.queryErr
}

func (d fakeDB) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return d.execTag, d.execErr
}

type captureDB struct {
	fakeDB
	sql  string
	args []any
}

func (d *captureDB) Exec(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	d.sql = sql
	d.args = args
	return d.execTag, d.execErr
}

func TestRepositoryListRequiresDatabase(t *testing.T) {
	if _, _, err := NewRepository(nil).List(context.Background(), Filter{Limit: 20}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("List error = %v, want ErrUnavailable", err)
	}
}

func TestRepositoryListRejectsInvalidFilters(t *testing.T) {
	tests := []struct {
		name   string
		filter Filter
	}{
		{name: "zero limit", filter: Filter{Limit: 0}},
		{name: "limit too large", filter: Filter{Limit: MaxPageSize + 1}},
		{name: "negative offset", filter: Filter{Limit: 20, Offset: -1}},
		{name: "unknown status", filter: Filter{Limit: 20, Status: "UNKNOWN"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, _, err := NewRepository(fakeDB{}).List(context.Background(), test.filter)
			if !errors.Is(err, ErrInvalidFilter) {
				t.Fatalf("List error = %v, want ErrInvalidFilter", err)
			}
		})
	}
}

func TestRepositoryListHidesDriverErrors(t *testing.T) {
	driverError := errors.New("pq: SELECT recipient FROM notification.notification_logs host=10.0.0.5")

	t.Run("count", func(t *testing.T) {
		_, _, err := NewRepository(fakeDB{row: fakeRow{err: driverError}}).List(context.Background(), Filter{Limit: 20})
		if !errors.Is(err, ErrListFailed) {
			t.Fatalf("List error = %v, want ErrListFailed", err)
		}
		if strings.Contains(err.Error(), "SELECT") || strings.Contains(err.Error(), "10.0.0.5") {
			t.Fatalf("repository leaked driver details: %v", err)
		}
	})

	t.Run("page", func(t *testing.T) {
		_, _, err := NewRepository(fakeDB{row: fakeRow{total: 3}, queryErr: driverError}).List(context.Background(), Filter{Limit: 20})
		if !errors.Is(err, ErrListFailed) {
			t.Fatalf("List error = %v, want ErrListFailed", err)
		}
		if strings.Contains(err.Error(), "SELECT") || strings.Contains(err.Error(), "10.0.0.5") {
			t.Fatalf("repository leaked driver details: %v", err)
		}
	})
}

func TestValidStatus(t *testing.T) {
	for _, status := range []string{idempotency.StatusPending, idempotency.StatusSent, idempotency.StatusFailed} {
		if !ValidStatus(status) {
			t.Fatalf("ValidStatus(%q) = false, want true", status)
		}
	}
	for _, status := range []string{"", "pending", "UNKNOWN"} {
		if ValidStatus(status) {
			t.Fatalf("ValidStatus(%q) = true, want false", status)
		}
	}
}

func TestDeleteBeforeRequiresDatabase(t *testing.T) {
	if _, err := NewRepository(nil).DeleteBefore(context.Background(), time.Now()); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("DeleteBefore error = %v, want ErrUnavailable", err)
	}
}

func TestDeleteBeforeRejectsZeroCutoff(t *testing.T) {
	if _, err := NewRepository(fakeDB{}).DeleteBefore(context.Background(), time.Time{}); !errors.Is(err, ErrInvalidCutoff) {
		t.Fatalf("DeleteBefore error = %v, want ErrInvalidCutoff", err)
	}
}

func TestDeleteBeforeUsesUTCAndOnlyDeletesDeliveryLogs(t *testing.T) {
	db := &captureDB{fakeDB: fakeDB{execTag: pgconn.NewCommandTag("DELETE 2")}}
	cutoff := time.Date(2026, time.November, 16, 12, 0, 0, 0, time.FixedZone("test", 5*60*60))

	deleted, err := NewRepository(db).DeleteBefore(context.Background(), cutoff)
	if err != nil {
		t.Fatalf("DeleteBefore returned error: %v", err)
	}
	if deleted != 2 {
		t.Fatalf("deleted = %d, want 2", deleted)
	}
	if len(db.args) != 1 {
		t.Fatalf("args = %d, want 1", len(db.args))
	}
	arg, ok := db.args[0].(time.Time)
	if !ok {
		t.Fatalf("cutoff arg = %T, want time.Time", db.args[0])
	}
	if arg != cutoff.UTC() {
		t.Fatalf("cutoff arg = %v, want %v", arg, cutoff.UTC())
	}
	if arg.Location() != time.UTC {
		t.Fatalf("cutoff location = %v, want UTC", arg.Location())
	}
	if !strings.Contains(db.sql, "notification.notification_logs") {
		t.Fatalf("delete SQL does not target notification_logs: %s", db.sql)
	}
	for _, forbidden := range []string{"processed_events", "merchant."} {
		if strings.Contains(db.sql, forbidden) {
			t.Fatalf("delete SQL targets %s: %s", forbidden, db.sql)
		}
	}
}

func TestDeleteBeforeHidesDriverErrors(t *testing.T) {
	driverError := errors.New("pq: DELETE FROM notification.notification_logs host=10.0.0.5")
	_, err := NewRepository(fakeDB{execErr: driverError}).DeleteBefore(context.Background(), time.Now().UTC())
	if !errors.Is(err, ErrCleanupFailed) {
		t.Fatalf("DeleteBefore error = %v, want ErrCleanupFailed", err)
	}
	if strings.Contains(err.Error(), "DELETE") || strings.Contains(err.Error(), "10.0.0.5") {
		t.Fatalf("repository leaked driver details: %v", err)
	}
}
