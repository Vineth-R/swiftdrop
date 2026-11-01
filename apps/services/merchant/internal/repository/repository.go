package repository

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var ErrNotFound = errors.New("merchant resource not found")
var ErrInvalidState = errors.New("invalid merchant state transition")

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
