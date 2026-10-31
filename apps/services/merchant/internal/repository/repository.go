package repository

import (
	"database/sql"
	"errors"
)

var ErrNotFound = errors.New("merchant resource not found")
var ErrInvalidState = errors.New("invalid merchant state transition")

// Repository owns all SQL access to the merchant schema.
type Repository struct {
	db *sql.DB
}

func New(db *sql.DB) (*Repository, error) {
	if db == nil {
		return nil, errors.New("merchant repository requires a database")
	}

	return &Repository{db: db}, nil
}
