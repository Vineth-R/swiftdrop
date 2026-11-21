package repository

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"time"

	"github.com/jackc/pgx/v5"
)

// ErrResetTokenInvalid covers an unknown, expired or already used password
// reset token, and a token whose user is deactivated. The caller cannot tell
// these apart (API v1.1 section 2.17, 400 INVALID_OR_EXPIRED_RESET_TOKEN).
var ErrResetTokenInvalid = errors.New("password reset token is not valid")

// NewPasswordResetToken describes a reset token to store. Only the SHA-256 hash
// of the token is persisted, never the token itself (DB v1.1 section 3.7).
type NewPasswordResetToken struct {
	UserID      string
	TokenHash   string
	ExpiresAt   time.Time
	RequestedIP *netip.Addr
}

// IssuePasswordResetToken stores a new reset token for the user. In one
// transaction it first marks the user's earlier unused tokens as used, so only
// the newest link works, then inserts the new one.
func (r *Repository) IssuePasswordResetToken(ctx context.Context, token NewPasswordResetToken, now time.Time) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("issuing password reset token: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Lock the user row so two simultaneous requests for the same account run
	// one after the other; otherwise both could insert an unused token and hit
	// the one-unused-token-per-user index.
	var locked string
	err = tx.QueryRow(ctx, `SELECT id FROM identity.users WHERE id = $1 FOR UPDATE`, token.UserID).Scan(&locked)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("issuing password reset token: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		UPDATE identity.password_reset_tokens
		SET used_at = $2
		WHERE user_id = $1 AND used_at IS NULL`,
		token.UserID, now.UTC(),
	); err != nil {
		return fmt.Errorf("issuing password reset token: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO identity.password_reset_tokens (user_id, token_hash, expires_at, requested_ip)
		VALUES ($1, $2, $3, $4)`,
		token.UserID, token.TokenHash, token.ExpiresAt.UTC(), token.RequestedIP,
	); err != nil {
		return fmt.Errorf("issuing password reset token: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("issuing password reset token: %w", err)
	}
	return nil
}

// PasswordResetTokenUsable reports whether the reset token with hash tokenHash
// could be used right now: it exists, is unused, is unexpired and belongs to an
// active user. It does not spend the token. It lets the caller check the token
// before the new password, as API v1.1 section 2.17 orders the checks;
// ResetPasswordWithToken repeats the check under a lock when it spends the
// token, so this answer is never relied on for the change itself.
func (r *Repository) PasswordResetTokenUsable(ctx context.Context, tokenHash string, now time.Time) (bool, error) {
	var usable bool
	err := r.db.QueryRow(ctx, `
		SELECT t.used_at IS NULL AND t.expires_at > $2 AND u.is_active
		FROM identity.password_reset_tokens t
		JOIN identity.users u ON u.id = t.user_id
		WHERE t.token_hash = $1`,
		tokenHash, now.UTC(),
	).Scan(&usable)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("checking password reset token: %w", err)
	}
	return usable, nil
}

// ResetPasswordWithToken consumes the reset token with hash tokenHash and sets
// the user's password hash, all in one transaction: the password changes, the
// token is marked used, any login lock is cleared, and every active refresh
// token of the user is revoked so old sessions stop working. It returns the
// user's ID, or ErrResetTokenInvalid when the token is unknown, expired, used,
// or its user is deactivated.
//
// The token row is locked, so two requests with the same token cannot both
// succeed.
func (r *Repository) ResetPasswordWithToken(
	ctx context.Context,
	tokenHash, newPasswordHash string,
	now time.Time,
) (string, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("resetting password: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var (
		tokenID   string
		userID    string
		expiresAt time.Time
		usedAt    *time.Time
		isActive  bool
	)
	err = tx.QueryRow(ctx, `
		SELECT t.id, t.user_id, t.expires_at, t.used_at, u.is_active
		FROM identity.password_reset_tokens t
		JOIN identity.users u ON u.id = t.user_id
		WHERE t.token_hash = $1
		FOR UPDATE OF t`,
		tokenHash,
	).Scan(&tokenID, &userID, &expiresAt, &usedAt, &isActive)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrResetTokenInvalid
	}
	if err != nil {
		return "", fmt.Errorf("resetting password: %w", err)
	}
	if usedAt != nil || !now.Before(expiresAt) || !isActive {
		return "", ErrResetTokenInvalid
	}

	if _, err := tx.Exec(ctx, `
		UPDATE identity.users
		SET password_hash = $2, failed_login_count = 0, locked_until = NULL
		WHERE id = $1`,
		userID, newPasswordHash,
	); err != nil {
		return "", fmt.Errorf("resetting password: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE identity.password_reset_tokens SET used_at = $2 WHERE id = $1`, tokenID, now.UTC()); err != nil {
		return "", fmt.Errorf("resetting password: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE identity.refresh_tokens
		SET revoked_at = $2
		WHERE user_id = $1 AND revoked_at IS NULL`,
		userID, now.UTC(),
	); err != nil {
		return "", fmt.Errorf("resetting password: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("resetting password: %w", err)
	}
	return userID, nil
}
