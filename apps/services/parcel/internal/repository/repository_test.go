package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestNewRequiresDatabase(t *testing.T) {
	repository, err := New(nil)
	if err == nil || repository != nil {
		t.Fatal("New(nil) should return an error and no repository")
	}
}

func TestNewAcceptsDatabase(t *testing.T) {
	database := &fakeDB{}
	repository, err := New(database)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	if repository.db != database {
		t.Fatal("New() did not retain the supplied database")
	}
}

func TestDatabaseError(t *testing.T) {
	tests := []struct {
		name       string
		code       string
		constraint string
		want       error
	}{
		{
			name: "tracking collision", code: "23505",
			constraint: "parcels_tracking_number_key", want: ErrDuplicateTrackingNumber,
		},
		{
			name: "customer duplicate", code: "23505",
			constraint: "parcels_customer_idem_uq", want: ErrDuplicateIdempotencyKey,
		},
		{
			name: "merchant duplicate", code: "23505",
			constraint: "parcels_merchant_external_uq", want: ErrDuplicateMerchantOrder,
		},
		{name: "missing required value", code: "23502", want: ErrInvalidData},
		{name: "history foreign key", code: "23503", want: ErrInvalidData},
		{name: "owner check", code: "23514", want: ErrInvalidData},
		{name: "text too long", code: "22001", want: ErrInvalidData},
		{name: "invalid uuid", code: "22P02", want: ErrInvalidData},
		{name: "numeric overflow", code: "22003", want: ErrInvalidData},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			postgresError := &pgconn.PgError{
				Code: test.code, ConstraintName: test.constraint,
				Message: "private supplied value", Detail: "private supplied detail",
			}
			err := databaseError(fmt.Errorf("driver: %w", postgresError))
			if !errors.Is(err, test.want) {
				t.Fatalf("databaseError() = %v, want %v", err, test.want)
			}
			if strings.Contains(err.Error(), "private") {
				t.Fatal("expected database errors must not expose supplied values")
			}
		})
	}
}

func TestDatabaseErrorPreservesUnexpectedFailures(t *testing.T) {
	unknown := errors.New("connection lost")
	for _, err := range []error{context.Canceled, context.DeadlineExceeded, unknown} {
		if !errors.Is(databaseError(err), err) {
			t.Fatalf("databaseError() did not preserve %v", err)
		}
	}
	if !errors.Is(databaseError(fmt.Errorf("scan: %w", pgx.ErrNoRows)), ErrNotFound) {
		t.Fatal("a wrapped pgx.ErrNoRows should become ErrNotFound")
	}
}

type fakeDB struct {
	row      pgx.Row
	rows     pgx.Rows
	queryErr error
	context  context.Context
}

func (db *fakeDB) QueryRow(ctx context.Context, _ string, _ ...any) pgx.Row {
	db.context = ctx
	return db.row
}

func (db *fakeDB) Query(ctx context.Context, _ string, _ ...any) (pgx.Rows, error) {
	db.context = ctx
	return db.rows, db.queryErr
}

type errorRow struct {
	err error
}

func (row errorRow) Scan(_ ...any) error {
	return row.err
}
