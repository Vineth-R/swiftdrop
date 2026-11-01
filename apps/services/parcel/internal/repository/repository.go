package repository

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var (
	ErrNotFound                = errors.New("parcel resource not found")
	ErrDuplicateTrackingNumber = errors.New("parcel tracking number already exists")
	ErrDuplicateIdempotencyKey = errors.New("customer idempotency key already exists")
	ErrDuplicateMerchantOrder  = errors.New("merchant external order already exists")
	ErrInvalidData             = errors.New("parcel data violates a database constraint")
)

// DB is implemented by the shared postgres.Pool and by pgx.Tx.
type DB interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

// Repository owns persistence for parcels and their status history. Construct it
// with a pgx.Tx for writes that must commit with history, assignments or outbox
// records. The caller owns that transaction; repository methods never commit it.
type Repository struct {
	db DB
}

func New(db DB) (*Repository, error) {
	if db == nil {
		return nil, errors.New("parcel repository requires a database")
	}

	return &Repository{db: db}, nil
}

func databaseError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}

	var postgresError *pgconn.PgError
	if !errors.As(err, &postgresError) {
		return err
	}

	if postgresError.Code == "23505" {
		switch postgresError.ConstraintName {
		case "parcels_tracking_number_key":
			return ErrDuplicateTrackingNumber
		case "parcels_customer_idem_uq":
			return ErrDuplicateIdempotencyKey
		case "parcels_merchant_external_uq":
			return ErrDuplicateMerchantOrder
		}
	}

	switch postgresError.Code {
	case "23502", "23503", "23514", "22001", "22P02", "22003":
		// PostgreSQL details can contain names, addresses or other supplied values.
		// Expected validation errors expose only a stable repository error.
		return ErrInvalidData
	}

	return err
}
