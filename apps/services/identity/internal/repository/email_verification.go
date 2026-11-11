package repository

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"time"

	"github.com/jackc/pgx/v5"
)

// ErrVerificationTokenInvalid covers an unknown, expired or already used email
// verification token, and a token whose user is inactive.
var ErrVerificationTokenInvalid = errors.New("email verification token is not valid")

// NewEmailVerificationToken describes a token to store. Only the SHA-256 hash
// is persisted, never the token itself.
type NewEmailVerificationToken struct {
	UserID      string
	TokenHash   string
	ExpiresAt   time.Time
	RequestedIP *netip.Addr
}

// CreateUserWithVerificationToken inserts an unverified LOCAL account and its
// verification token in one transaction.
func (r *Repository) CreateUserWithVerificationToken(
	ctx context.Context,
	params CreateUserParams,
	token NewEmailVerificationToken,
) (User, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return User{}, fmt.Errorf("creating user: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

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
	err = tx.QueryRow(ctx, `
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
	if _, err := tx.Exec(ctx, `
		INSERT INTO identity.email_verification_tokens (user_id, token_hash, expires_at, requested_ip)
		VALUES ($1, $2, $3, $4)`,
		user.ID, token.TokenHash, token.ExpiresAt.UTC(), token.RequestedIP,
	); err != nil {
		return User{}, fmt.Errorf("creating verification token: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return User{}, fmt.Errorf("creating user: %w", err)
	}
	return user, nil
}

// IssueEmailVerificationToken invalidates the user's earlier unused tokens and
// inserts a new one in a single transaction.
func (r *Repository) IssueEmailVerificationToken(ctx context.Context, token NewEmailVerificationToken, now time.Time) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("issuing verification token: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var locked string
	err = tx.QueryRow(ctx, `SELECT id FROM identity.users WHERE id = $1 FOR UPDATE`, token.UserID).Scan(&locked)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("issuing verification token: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE identity.email_verification_tokens
		SET used_at = $2
		WHERE user_id = $1 AND used_at IS NULL`,
		token.UserID, now.UTC(),
	); err != nil {
		return fmt.Errorf("issuing verification token: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO identity.email_verification_tokens (user_id, token_hash, expires_at, requested_ip)
		VALUES ($1, $2, $3, $4)`,
		token.UserID, token.TokenHash, token.ExpiresAt.UTC(), token.RequestedIP,
	); err != nil {
		return fmt.Errorf("issuing verification token: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("issuing verification token: %w", err)
	}
	return nil
}

// ConsumeEmailVerificationToken marks the token used and the user's email
// verified in one transaction. It returns the user ID.
func (r *Repository) ConsumeEmailVerificationToken(ctx context.Context, tokenHash string, now time.Time) (string, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("verifying email: %w", err)
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
		FROM identity.email_verification_tokens t
		JOIN identity.users u ON u.id = t.user_id
		WHERE t.token_hash = $1
		FOR UPDATE OF t`,
		tokenHash,
	).Scan(&tokenID, &userID, &expiresAt, &usedAt, &isActive)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrVerificationTokenInvalid
	}
	if err != nil {
		return "", fmt.Errorf("verifying email: %w", err)
	}
	if usedAt != nil || !now.Before(expiresAt) || !isActive {
		return "", ErrVerificationTokenInvalid
	}
	if _, err := tx.Exec(ctx, `UPDATE identity.users SET email_verified = true WHERE id = $1`, userID); err != nil {
		return "", fmt.Errorf("verifying email: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE identity.email_verification_tokens SET used_at = $2 WHERE id = $1`, tokenID, now.UTC()); err != nil {
		return "", fmt.Errorf("verifying email: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("verifying email: %w", err)
	}
	return userID, nil
}
