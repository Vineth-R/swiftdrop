package repository

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestRepository_internalQueries_errorMapping(t *testing.T) {
	boom := errors.New("connection reset")
	call := map[string]func(*Repository) error{
		"CourierEligibility": func(r *Repository) error {
			_, err := r.CourierEligibility(context.Background(), "id")
			return err
		},
		"UserContact": func(r *Repository) error {
			_, err := r.UserContact(context.Background(), "id")
			return err
		},
	}

	for name, run := range call {
		t.Run(name+" no rows is ErrNotFound", func(t *testing.T) {
			repo, _ := New(&fakeDB{row: fakeRow{err: pgx.ErrNoRows}})
			if err := run(repo); !errors.Is(err, ErrNotFound) {
				t.Fatalf("error = %v, want ErrNotFound", err)
			}
		})
		t.Run(name+" database failure is not ErrNotFound", func(t *testing.T) {
			repo, _ := New(&fakeDB{row: fakeRow{err: boom}})
			err := run(repo)
			if !errors.Is(err, boom) || errors.Is(err, ErrNotFound) {
				t.Fatalf("error = %v, want the database error and not ErrNotFound", err)
			}
		})
	}
}
