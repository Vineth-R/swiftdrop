package notificationlog

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

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
}

func (d fakeDB) QueryRow(context.Context, string, ...any) pgx.Row {
	return d.row
}

func (d fakeDB) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, d.queryErr
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
