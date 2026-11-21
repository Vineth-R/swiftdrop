package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/Vineth-R/swiftdrop/apps/services/identity/internal/lockout"
	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
)

const (
	uniqueViolation  = "23505"
	emailUniqueIndex = "users_email_lower_uq"
)

// User is an identity.users row joined with its role name.
type User struct {
	ID               string
	Email            string
	PasswordHash     *string
	FullName         string
	Phone            *string
	Role             auth.Role
	AuthProvider     string
	EmailVerified    bool
	SessionVersion   int
	IsActive         bool
	FailedLoginCount int
	LockedUntil      *time.Time
	CreatedAt        time.Time
}

// LockoutState returns the persisted lockout state of the user.
func (u User) LockoutState() lockout.State {
	return lockout.State{FailedCount: u.FailedLoginCount, LockedUntil: u.LockedUntil}
}

// CreateUserParams describes a new local (email and password) account.
type CreateUserParams struct {
	Email        string
	PasswordHash string
	FullName     string
	Phone        string
	Role         auth.Role
}

const userColumns = `u.id, u.email, u.password_hash, u.full_name, u.phone, r.name,
	u.auth_provider, u.email_verified, u.session_version, u.is_active, u.failed_login_count, u.locked_until, u.created_at`

// CreateUser inserts a LOCAL account. It returns ErrEmailTaken when the email is
// already registered in any letter case.
func (r *Repository) CreateUser(ctx context.Context, params CreateUserParams) (User, error) {
	user := User{
		Email:         params.Email,
		PasswordHash:  &params.PasswordHash,
		FullName:      params.FullName,
		Phone:         &params.Phone,
		Role:          params.Role,
		AuthProvider:  "LOCAL",
		EmailVerified: false,
		IsActive:      true,
	}

	err := r.db.QueryRow(ctx, `
		INSERT INTO identity.users (email, password_hash, full_name, phone, role_id, auth_provider)
		SELECT $1, $2, $3, $4, ro.id, 'LOCAL'
		FROM identity.roles ro
		WHERE ro.name = $5
		RETURNING id, created_at`,
		params.Email, params.PasswordHash, params.FullName, params.Phone, string(params.Role),
	).Scan(&user.ID, &user.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, fmt.Errorf("creating user: unknown role %q", params.Role)
	}
	if isEmailTaken(err) {
		return User{}, ErrEmailTaken
	}
	if err != nil {
		return User{}, fmt.Errorf("creating user: %w", err)
	}
	return user, nil
}

// UserByEmail finds a user by email, ignoring letter case.
func (r *Repository) UserByEmail(ctx context.Context, email string) (User, error) {
	return r.queryUser(ctx, "lower(u.email) = lower($1)", email)
}

// UserByID finds a user by primary key.
func (r *Repository) UserByID(ctx context.Context, userID string) (User, error) {
	return r.queryUser(ctx, "u.id = $1", userID)
}

func (r *Repository) queryUser(ctx context.Context, condition string, arg any) (User, error) {
	user := User{}
	var role string
	err := r.db.QueryRow(ctx, `
		SELECT `+userColumns+`
		FROM identity.users u
		JOIN identity.roles r ON r.id = u.role_id
		WHERE `+condition,
		arg,
	).Scan(
		&user.ID,
		&user.Email,
		&user.PasswordHash,
		&user.FullName,
		&user.Phone,
		&role,
		&user.AuthProvider,
		&user.EmailVerified,
		&user.SessionVersion,
		&user.IsActive,
		&user.FailedLoginCount,
		&user.LockedUntil,
		&user.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, fmt.Errorf("querying user: %w", err)
	}
	user.Role = auth.Role(role)
	return user, nil
}

// RecordSuccessfulLogin clears the failure counter and any lock, and stamps
// last_login_at.
func (r *Repository) RecordSuccessfulLogin(ctx context.Context, userID string, now time.Time) error {
	now = now.UTC()
	tag, err := r.db.Exec(ctx, `
		UPDATE identity.users
		SET failed_login_count = 0, locked_until = NULL, last_login_at = $2
		WHERE id = $1`,
		userID, now,
	)
	if err != nil {
		return fmt.Errorf("recording successful login: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// RecordFailedLogin applies policy to the user's stored lockout state and
// returns the new state. The row is locked for the read-modify-write, so
// concurrent failures cannot undercount.
func (r *Repository) RecordFailedLogin(
	ctx context.Context,
	userID string,
	policy lockout.Policy,
	now time.Time,
) (lockout.State, error) {
	now = now.UTC()
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return lockout.State{}, fmt.Errorf("recording failed login: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	current := lockout.State{}
	err = tx.QueryRow(ctx, `
		SELECT failed_login_count, locked_until
		FROM identity.users
		WHERE id = $1
		FOR UPDATE`,
		userID,
	).Scan(&current.FailedCount, &current.LockedUntil)
	if errors.Is(err, pgx.ErrNoRows) {
		return lockout.State{}, ErrNotFound
	}
	if err != nil {
		return lockout.State{}, fmt.Errorf("recording failed login: %w", err)
	}

	next := policy.AfterFailure(current, now)
	if _, err := tx.Exec(ctx, `
		UPDATE identity.users
		SET failed_login_count = $2, locked_until = $3
		WHERE id = $1`,
		userID, next.FailedCount, next.LockedUntil,
	); err != nil {
		return lockout.State{}, fmt.Errorf("recording failed login: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return lockout.State{}, fmt.Errorf("recording failed login: %w", err)
	}
	return next, nil
}

func isEmailTaken(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == uniqueViolation && pgErr.ConstraintName == emailUniqueIndex
}
