// Package repository owns all SQL access to the identity schema. Nothing else
// in the identity service writes SQL, and no other service reads these tables
// (DB v1.1 section 1).
package repository

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var (
	// ErrNotFound means the requested row does not exist.
	ErrNotFound = errors.New("identity resource not found")

	// ErrEmailTaken means an account already uses the email (case-insensitive).
	ErrEmailTaken = errors.New("email is already registered")

	// ErrRefreshTokenInvalid covers an unknown, expired, revoked or already
	// rotated refresh token, and a token whose user is deactivated. The caller
	// cannot tell these apart (API v1.1 section 2.3).
	ErrRefreshTokenInvalid = errors.New("refresh token is not valid")

	// ErrPasswordMismatch means the stored password hash is no longer the one
	// the caller checked, because the password was changed in the meantime.
	ErrPasswordMismatch = errors.New("stored password hash changed")
)

// DB is the subset of the shared PostgreSQL pool the repository needs.
// *postgres.Pool satisfies it.
type DB interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	Begin(ctx context.Context) (pgx.Tx, error)
}

// Repository is the identity data-access layer.
type Repository struct {
	db DB
}

// New returns a Repository that uses db.
func New(db DB) (*Repository, error) {
	if db == nil {
		return nil, errors.New("identity repository requires a database")
	}
	return &Repository{db: db}, nil
}
