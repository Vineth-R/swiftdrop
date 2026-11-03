package repository

import (
	"context"
	"fmt"
	"time"
)

// ProfileChange is a partial update of a user's own name and phone. A nil field
// is left unchanged.
type ProfileChange struct {
	FullName *string
	Phone    *string
}

// UpdateProfile applies change to the user and returns the account as stored. It
// returns ErrNotFound when the user does not exist.
func (r *Repository) UpdateProfile(ctx context.Context, userID string, change ProfileChange) (User, error) {
	tag, err := r.db.Exec(ctx, `
		UPDATE identity.users
		SET full_name = COALESCE($2, full_name), phone = COALESCE($3, phone)
		WHERE id = $1`,
		userID, change.FullName, change.Phone,
	)
	if err != nil {
		return User{}, fmt.Errorf("updating profile: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return User{}, ErrNotFound
	}
	return r.UserByID(ctx, userID)
}

// ChangePassword replaces the user's password hash, clears any login lock and
// revokes every refresh token of the user, all in one transaction. A user
// changing their password is signed out everywhere.
//
// It only succeeds while the stored hash is still expectedHash, the hash the
// caller verified, so two requests that both knew the old password cannot both
// win. It returns ErrPasswordMismatch when the hash changed or the user has no
// password, and ErrNotFound when the user does not exist.
func (r *Repository) ChangePassword(
	ctx context.Context,
	userID, expectedHash, newHash string,
	now time.Time,
) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("changing password: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	tag, err := tx.Exec(ctx, `
		UPDATE identity.users
		SET password_hash = $3, failed_login_count = 0, locked_until = NULL
		WHERE id = $1 AND password_hash = $2`,
		userID, expectedHash, newHash,
	)
	if err != nil {
		return fmt.Errorf("changing password: %w", err)
	}
	if tag.RowsAffected() == 0 {
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM identity.users WHERE id = $1)`, userID).Scan(&exists); err != nil {
			return fmt.Errorf("changing password: %w", err)
		}
		if !exists {
			return ErrNotFound
		}
		return ErrPasswordMismatch
	}

	if _, err := tx.Exec(ctx, `
		UPDATE identity.refresh_tokens
		SET revoked_at = $2
		WHERE user_id = $1 AND revoked_at IS NULL`,
		userID, now.UTC(),
	); err != nil {
		return fmt.Errorf("changing password: revoking sessions: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("changing password: %w", err)
	}
	return nil
}
