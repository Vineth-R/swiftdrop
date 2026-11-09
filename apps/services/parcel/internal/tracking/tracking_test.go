package tracking

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

type fixtureDB struct {
	status string
	err    error
}
type fixtureRow struct {
	status string
	err    error
}

func (db fixtureDB) QueryRow(context.Context, string, ...any) pgx.Row {
	return fixtureRow{status: db.status, err: db.err}
}
func (row fixtureRow) Scan(dest ...any) error {
	if row.err != nil {
		return row.err
	}
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	values := []any{"SDK7M2Q9XP4", row.status, "Colombo", "Kandy", at, []byte(`[{"status":"CREATED","at":"2026-01-01T00:00:00Z"}]`)}
	for index, value := range values {
		reflect.ValueOf(dest[index]).Elem().Set(reflect.ValueOf(value))
	}
	return nil
}
func TestLookupPublicProjection(t *testing.T) {
	for _, status := range []string{"CREATED", "PICKED_UP", "IN_TRANSIT", "OUT_FOR_DELIVERY", "DELIVERED", "FAILED"} {
		t.Run(status, func(t *testing.T) {
			result, err := NewRepository(fixtureDB{status: status}).Lookup(t.Context(), "SDK7M2Q9XP4")
			if err != nil {
				t.Fatal(err)
			}
			if result.Status != status || len(result.Timeline) != 1 {
				t.Fatal(result)
			}
			raw, err := json.Marshal(result)
			if err != nil {
				t.Fatal(err)
			}
			for _, private := range []string{"name", "phone", "address", "note", "failureReason", "photo"} {
				if strings.Contains(string(raw), `"`+private+`"`) {
					t.Fatal("private field present")
				}
			}
		})
	}
}

type recordingDB struct {
	query string
	args  []any
	err   error
}

func (db *recordingDB) QueryRow(_ context.Context, query string, args ...any) pgx.Row {
	db.query = query
	db.args = args
	return fixtureRow{err: db.err}
}

func TestLookupAuthorizedScopesByOwner(t *testing.T) {
	for _, test := range []struct {
		name  string
		user  string
		staff bool
	}{
		{name: "customer", user: "customer-a"},
		{name: "staff", user: "staff-1", staff: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			db := &recordingDB{}
			result, err := NewRepository(db).LookupAuthorized(context.Background(), "SDK7M2Q9XP4", test.user, test.staff)
			if err != nil {
				t.Fatal(err)
			}
			if result.TrackingNumber != "SDK7M2Q9XP4" {
				t.Fatalf("result = %+v", result)
			}
			if !strings.Contains(db.query, "p.customer_user_id::text=$2") {
				t.Fatalf("lookup is not ownership-scoped: %s", db.query)
			}
			if len(db.args) != 3 || db.args[0] != "SDK7M2Q9XP4" || db.args[1] != test.user || db.args[2] != test.staff {
				t.Fatalf("args = %#v", db.args)
			}
		})
	}
}

func TestLookupAuthorizedFailures(t *testing.T) {
	if _, err := NewRepository(nil).LookupAuthorized(context.Background(), "SDK7M2Q9XP4", "customer-a", false); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("nil database = %v, want ErrUnavailable", err)
	}
	if _, err := NewRepository(&recordingDB{}).LookupAuthorized(context.Background(), "bad", "customer-a", false); !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid number = %v, want ErrInvalid", err)
	}
	if _, err := NewRepository(&recordingDB{}).LookupAuthorized(context.Background(), "SDK7M2Q9XP4", "  ", false); !errors.Is(err, ErrNotFound) {
		t.Fatalf("empty customer = %v, want ErrNotFound", err)
	}
	if _, err := NewRepository(&recordingDB{err: pgx.ErrNoRows}).LookupAuthorized(context.Background(), "SDK7M2Q9XP4", "other", false); !errors.Is(err, ErrNotFound) {
		t.Fatalf("no row = %v, want ErrNotFound", err)
	}
}

func TestLookupFailures(t *testing.T) {
	for _, test := range []struct {
		name, number string
		dbErr, want  error
	}{
		{name: "invalid", number: "bad", want: ErrInvalid},
		{name: "unknown", number: "SDK00000000", dbErr: pgx.ErrNoRows, want: ErrNotFound},
		{name: "database", number: "SDK00000000", dbErr: context.DeadlineExceeded, want: context.DeadlineExceeded},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := NewRepository(fixtureDB{err: test.dbErr}).Lookup(t.Context(), test.number)
			if !errors.Is(err, test.want) {
				t.Fatalf("error %v, want %v", err, test.want)
			}
		})
	}
	if _, err := NewRepository(nil).Lookup(t.Context(), "SDK00000000"); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
}
