package repository

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestParcelOperationsPropagateErrorsAndContext(t *testing.T) {
	operations := []struct {
		name string
		run  func(context.Context, *Repository) error
	}{
		{name: "create", run: func(ctx context.Context, repository *Repository) error {
			_, err := repository.CreateParcel(ctx, CreateParcelParams{})
			return err
		}},
		{name: "by id", run: func(ctx context.Context, repository *Repository) error {
			_, err := repository.ParcelByID(ctx, "parcel")
			return err
		}},
		{name: "customer replay", run: func(ctx context.Context, repository *Repository) error {
			_, err := repository.ParcelByCustomerIdempotencyKey(ctx, "customer", "key")
			return err
		}},
		{name: "merchant replay", run: func(ctx context.Context, repository *Repository) error {
			_, err := repository.ParcelByMerchantExternalOrderID(ctx, "merchant", "order")
			return err
		}},
	}
	tests := []struct {
		name string
		err  error
		want error
	}{
		{name: "missing", err: pgx.ErrNoRows, want: ErrNotFound},
		{name: "cancelled", err: context.Canceled, want: context.Canceled},
		{name: "timeout", err: context.DeadlineExceeded, want: context.DeadlineExceeded},
		{
			name: "duplicate customer key",
			err:  &pgconn.PgError{Code: "23505", ConstraintName: "parcels_customer_idem_uq"},
			want: ErrDuplicateIdempotencyKey,
		},
	}
	for _, operation := range operations {
		t.Run(operation.name, func(t *testing.T) {
			for _, test := range tests {
				t.Run(test.name, func(t *testing.T) {
					ctx, cancel := context.WithCancel(context.Background())
					defer cancel()
					database := &fakeDB{row: errorRow{err: test.err}}
					repository, err := New(database)
					if err != nil {
						t.Fatalf("New() error: %v", err)
					}
					if err := operation.run(ctx, repository); !errors.Is(err, test.want) {
						t.Fatalf("operation error = %v, want %v", err, test.want)
					}
					if database.context != ctx {
						t.Fatal("operation did not propagate its context")
					}
				})
			}
		})
	}
}
