package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
)

// ErrGoogleAccountConflict means the SwiftDrop account is already linked to a
// different Google account, or the Google account belongs to another user.
var ErrGoogleAccountConflict = errors.New("google account conflict")

const googleSubUniqueIndex = "users_google_sub_key"

// CreateGoogleUserParams describes a new account created by Google sign-in.
// Google has already verified the email.
type CreateGoogleUserParams struct {
	Email     string
	FullName  string
	GoogleSub string
	Role      auth.Role
}

// UserByGoogleSub finds a user by the Google account subject (google_sub).
func (r *Repository) UserByGoogleSub(ctx context.Context, googleSub string) (User, error) {
	return r.queryUser(ctx, "u.google_sub = $1", googleSub)
}

// CreateGoogleUser inserts a GOOGLE account: no password, email verified, no
// phone. It returns ErrEmailTaken when the email is already registered and
// ErrGoogleAccountConflict when the Google subject is already linked.
func (r *Repository) CreateGoogleUser(ctx context.Context, params CreateGoogleUserParams) (User, error) {
	user := User{
		Email:         params.Email,
		FullName:      params.FullName,
		Role:          params.Role,
		AuthProvider:  "GOOGLE",
		EmailVerified: true,
		IsActive:      true,
	}

	err := r.db.QueryRow(ctx, `
		INSERT INTO identity.users (email, password_hash, full_name, role_id, auth_provider, google_sub, email_verified)
		SELECT $1, NULL, $2, ro.id, 'GOOGLE', $3, true
		FROM identity.roles ro
		WHERE ro.name = $4
		RETURNING id, created_at`,
		params.Email, params.FullName, params.GoogleSub, string(params.Role),
	).Scan(&user.ID, &user.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, fmt.Errorf("creating google user: unknown role %q", params.Role)
	}
	if isEmailTaken(err) {
		return User{}, ErrEmailTaken
	}
	if isGoogleSubTaken(err) {
		return User{}, ErrGoogleAccountConflict
	}
	if err != nil {
		return User{}, fmt.Errorf("creating google user: %w", err)
	}
	return user, nil
}

// LinkGoogleAccount attaches a Google account to an existing user and marks the
// email verified, because Google proved the owner controls it. When the
// existing account's email was not verified, it also clears the existing
// password and revokes every active refresh token, so a password chosen by
// whoever pre-registered the address cannot be used afterwards. A verified
// account keeps its password.
//
// It returns passwordCleared, ErrGoogleAccountConflict if the user is already
// linked to a different Google account, and ErrNotFound if the user does not
// exist.
func (r *Repository) LinkGoogleAccount(ctx context.Context, userID, googleSub string, now time.Time) (bool, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("linking google account: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var (
		existingSub   *string
		emailVerified bool
	)
	err = tx.QueryRow(ctx, `
		SELECT google_sub, email_verified
		FROM identity.users
		WHERE id = $1
		FOR UPDATE`,
		userID,
	).Scan(&existingSub, &emailVerified)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, ErrNotFound
	}
	if err != nil {
		return false, fmt.Errorf("linking google account: %w", err)
	}
	if existingSub != nil {
		if *existingSub == googleSub {
			// Already linked to this Google identity; nothing to change.
			return false, nil
		}
		return false, ErrGoogleAccountConflict
	}

	clearPassword := !emailVerified
	if clearPassword {
		_, err = tx.Exec(ctx, `
			UPDATE identity.users
			SET google_sub = $2, email_verified = true, password_hash = NULL,
			    failed_login_count = 0, locked_until = NULL
			WHERE id = $1`,
			userID, googleSub,
		)
	} else {
		_, err = tx.Exec(ctx, `
			UPDATE identity.users
			SET google_sub = $2, email_verified = true,
			    failed_login_count = 0, locked_until = NULL
			WHERE id = $1`,
			userID, googleSub,
		)
	}
	if isGoogleSubTaken(err) {
		return false, ErrGoogleAccountConflict
	}
	if err != nil {
		return false, fmt.Errorf("linking google account: %w", err)
	}

	if clearPassword {
		if _, err := tx.Exec(ctx, `
			UPDATE identity.refresh_tokens
			SET revoked_at = $2
			WHERE user_id = $1 AND revoked_at IS NULL`,
			userID, now.UTC(),
		); err != nil {
			return false, fmt.Errorf("linking google account: revoking sessions: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("linking google account: %w", err)
	}
	return clearPassword, nil
}

// PurgeExpiredOAuthStates deletes states that can no longer be used, so
// abandoned login attempts do not pile up (DB v1.1 section 3.5).
func (r *Repository) PurgeExpiredOAuthStates(ctx context.Context, now time.Time) error {
	if _, err := r.db.Exec(ctx, `DELETE FROM identity.oauth_states WHERE expires_at <= $1`, now.UTC()); err != nil {
		return fmt.Errorf("purging oauth states: %w", err)
	}
	return nil
}

func isGoogleSubTaken(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == uniqueViolation && pgErr.ConstraintName == googleSubUniqueIndex
}
