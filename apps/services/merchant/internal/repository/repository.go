package repository

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var (
	ErrNotFound                  = errors.New("merchant resource not found")
	ErrInvalidState              = errors.New("invalid merchant state transition")
	ErrDuplicateApplication      = errors.New("a merchant application already exists for this email")
	ErrDuplicateBusiness         = errors.New("a merchant already uses this business registration number")
	ErrInvalidRegistrationNumber = errors.New("a valid business registration number is required")
	ErrRegistrationConflict      = errors.New("merchant registration conflicts with an existing record")
	// ErrWebhookSecretChanged means a concurrent rotation replaced the stored
	// ciphertext before a read-repair could update it. The caller must not retry
	// blindly; the new value is authoritative.
	ErrWebhookSecretChanged = errors.New("webhook secret changed concurrently")
)

// Repository owns all SQL access to the merchant schema.
type Repository struct {
	db DB
}

type DB interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	Begin(ctx context.Context) (pgx.Tx, error)
}

func New(db DB) (*Repository, error) {
	if db == nil {
		return nil, errors.New("merchant repository requires a database")
	}

	return &Repository{db: db}, nil
}
