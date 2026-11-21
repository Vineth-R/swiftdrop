package repository

import (
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/Vineth-R/swiftdrop/apps/services/identity/internal/lockout"
)

func TestUser_LockoutState(t *testing.T) {
	until := time.Date(2026, time.November, 2, 9, 45, 0, 0, time.UTC)
	user := User{FailedLoginCount: 5, LockedUntil: &until}

	got := user.LockoutState()
	if got.FailedCount != 5 || got.LockedUntil == nil || !got.LockedUntil.Equal(until) {
		t.Fatalf("LockoutState() = %+v, want count 5 locked until %v", got, until)
	}
	if got != (lockout.State{FailedCount: 5, LockedUntil: &until}) {
		t.Fatalf("LockoutState() = %+v is not the expected state", got)
	}
}

func TestIsEmailTaken(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "email unique index violation",
			err:  &pgconn.PgError{Code: "23505", ConstraintName: "users_email_lower_uq"},
			want: true,
		},
		{
			name: "wrapped violation",
			err:  errors.Join(errors.New("insert"), &pgconn.PgError{Code: "23505", ConstraintName: "users_email_lower_uq"}),
			want: true,
		},
		{
			name: "other unique violation",
			err:  &pgconn.PgError{Code: "23505", ConstraintName: "users_google_sub_key"},
			want: false,
		},
		{
			name: "other error code",
			err:  &pgconn.PgError{Code: "23502", ConstraintName: "users_email_lower_uq"},
			want: false,
		},
		{name: "plain error", err: errors.New("boom"), want: false},
		{name: "nil", err: nil, want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := isEmailTaken(test.err); got != test.want {
				t.Fatalf("isEmailTaken() = %t, want %t", got, test.want)
			}
		})
	}
}
