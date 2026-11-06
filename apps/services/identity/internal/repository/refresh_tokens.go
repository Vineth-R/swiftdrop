package repository

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const userAgentMaxLength = 255

// RefreshReuseGrace is how long after a rotation the old token may still be
// presented without being treated as stolen. A browser with two tabs, or a
// client that retries after a lost response, can legitimately send the old
// token again within moments; that request is refused but harmless. Presenting
// it later means someone kept a copy, so the whole chain is revoked.
const RefreshReuseGrace = 10 * time.Second

// NewRefreshToken describes a refresh token to store. Only the SHA-256 hash of
// the token is persisted, never the token itself (DB v1.1 section 3.4).
type NewRefreshToken struct {
	UserID    string
	TokenHash string
	ExpiresAt time.Time
	UserAgent string
	IPAddress *netip.Addr
}

// NextRefreshToken is the replacement stored when a refresh token is rotated.
// It belongs to the same user as the token it replaces.
type NextRefreshToken struct {
	TokenHash string
	ExpiresAt time.Time
	UserAgent string
	IPAddress *netip.Addr
}

// CreateRefreshToken stores a new refresh token and returns its record ID.
func (r *Repository) CreateRefreshToken(ctx context.Context, token NewRefreshToken) (string, error) {
	var id string
	err := r.db.QueryRow(ctx, `
		INSERT INTO identity.refresh_tokens (user_id, token_hash, expires_at, user_agent, ip_address)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id`,
		token.UserID, token.TokenHash, token.ExpiresAt.UTC(), truncateUserAgent(token.UserAgent), token.IPAddress,
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("creating refresh token: %w", err)
	}
	return id, nil
}

// RotateRefreshToken exchanges the token with hash oldHash for next in one
// transaction: the old token is revoked and points at its replacement. It
// returns the owning user's ID, or ErrRefreshTokenInvalid when the token is
// unknown, expired, revoked, already rotated, or its user is deactivated.
//
// The old row is locked, so two concurrent refreshes with the same token
// cannot both succeed.
//
// A token that was already rotated and is presented again more than
// RefreshReuseGrace later is treated as stolen: every token that descends from
// it is revoked, which ends the session the thief and the owner share. The
// error is then ErrRefreshTokenReused (still an ErrRefreshTokenInvalid) and the
// owner's ID is returned with it so the caller can log the event. Tokens of the
// same user from other logins are not touched.
func (r *Repository) RotateRefreshToken(
	ctx context.Context,
	oldHash string,
	next NextRefreshToken,
	now time.Time,
) (string, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("rotating refresh token: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var (
		oldID      string
		userID     string
		expiresAt  time.Time
		revokedAt  *time.Time
		replacedBy *string
		isActive   bool
	)
	err = tx.QueryRow(ctx, `
		SELECT t.id, t.user_id, t.expires_at, t.revoked_at, t.replaced_by, u.is_active
		FROM identity.refresh_tokens t
		JOIN identity.users u ON u.id = t.user_id
		WHERE t.token_hash = $1
		FOR UPDATE OF t`,
		oldHash,
	).Scan(&oldID, &userID, &expiresAt, &revokedAt, &replacedBy, &isActive)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrRefreshTokenInvalid
	}
	if err != nil {
		return "", fmt.Errorf("rotating refresh token: %w", err)
	}

	// Logout and a password change revoke a token without replacing it, so only
	// a token with a successor can have been copied during a rotation.
	if revokedAt != nil && replacedBy != nil && !now.Before(revokedAt.Add(RefreshReuseGrace)) {
		if err := revokeChain(ctx, tx, oldID, now); err != nil {
			return "", err
		}
		if err := tx.Commit(ctx); err != nil {
			return "", fmt.Errorf("rotating refresh token: %w", err)
		}
		return userID, ErrRefreshTokenReused
	}
	if revokedAt != nil || !now.Before(expiresAt) || !isActive {
		return "", ErrRefreshTokenInvalid
	}

	var newID string
	err = tx.QueryRow(ctx, `
		INSERT INTO identity.refresh_tokens (user_id, token_hash, expires_at, user_agent, ip_address)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id`,
		userID, next.TokenHash, next.ExpiresAt.UTC(), truncateUserAgent(next.UserAgent), next.IPAddress,
	).Scan(&newID)
	if err != nil {
		return "", fmt.Errorf("rotating refresh token: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		UPDATE identity.refresh_tokens
		SET revoked_at = $2, replaced_by = $3
		WHERE id = $1`,
		oldID, now.UTC(), newID,
	); err != nil {
		return "", fmt.Errorf("rotating refresh token: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("rotating refresh token: %w", err)
	}
	return userID, nil
}

// revokeChain revokes every token that descends from the token with ID rootID
// through replaced_by, that is the token issued when it was rotated, the one
// issued when that was rotated, and so on. A chain that is revoked while one of
// its tokens is being rotated by another request can leave that request's new
// token alive; the next use of the old token revokes it.
func revokeChain(ctx context.Context, tx pgx.Tx, rootID string, now time.Time) error {
	if _, err := tx.Exec(ctx, `
		WITH RECURSIVE chain AS (
			SELECT id, replaced_by FROM identity.refresh_tokens WHERE id = $1
			UNION ALL
			SELECT t.id, t.replaced_by
			FROM identity.refresh_tokens t
			JOIN chain c ON t.id = c.replaced_by
		)
		UPDATE identity.refresh_tokens
		SET revoked_at = $2
		WHERE id IN (SELECT id FROM chain) AND revoked_at IS NULL`,
		rootID, now.UTC(),
	); err != nil {
		return fmt.Errorf("revoking refresh token chain: %w", err)
	}
	return nil
}

// RevokeRefreshToken revokes the user's refresh token with hash tokenHash. It is
// idempotent: an unknown, foreign or already revoked token is not an error, so
// logout always succeeds.
func (r *Repository) RevokeRefreshToken(ctx context.Context, userID, tokenHash string, now time.Time) error {
	if _, err := r.db.Exec(ctx, `
		UPDATE identity.refresh_tokens
		SET revoked_at = $3
		WHERE user_id = $1 AND token_hash = $2 AND revoked_at IS NULL`,
		userID, tokenHash, now.UTC(),
	); err != nil {
		return fmt.Errorf("revoking refresh token: %w", err)
	}
	return nil
}

// truncateUserAgent fits a client-supplied User-Agent into the column. Invalid
// UTF-8 is dropped because PostgreSQL rejects it, which would fail the login.
func truncateUserAgent(value string) *string {
	value = strings.ToValidUTF8(value, "")
	if len(value) > userAgentMaxLength {
		value = strings.ToValidUTF8(value[:userAgentMaxLength], "")
	}
	if value == "" {
		return nil
	}
	return &value
}
