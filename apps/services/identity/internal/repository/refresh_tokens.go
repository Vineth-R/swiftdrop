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
		oldID     string
		userID    string
		expiresAt time.Time
		revokedAt *time.Time
		isActive  bool
	)
	err = tx.QueryRow(ctx, `
		SELECT t.id, t.user_id, t.expires_at, t.revoked_at, u.is_active
		FROM identity.refresh_tokens t
		JOIN identity.users u ON u.id = t.user_id
		WHERE t.token_hash = $1
		FOR UPDATE OF t`,
		oldHash,
	).Scan(&oldID, &userID, &expiresAt, &revokedAt, &isActive)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrRefreshTokenInvalid
	}
	if err != nil {
		return "", fmt.Errorf("rotating refresh token: %w", err)
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
