package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
	"github.com/jackc/pgx/v5"
)

var (
	ErrTransitionStale    = errors.New("lifecycle transition sequence is stale")
	ErrTransitionConflict = errors.New("lifecycle transition ID conflicts with an existing transition")
)

// SessionState is the current session authority state of a user.
type SessionState struct {
	Role           auth.Role
	IsActive       bool
	SessionVersion int
}

// MerchantLifecycleTransition is one applied identity lifecycle operation.
type MerchantLifecycleTransition struct {
	TransitionID    string
	UserID          string
	Sequence        int64
	Operation       string
	AlreadyApplied  bool
	SessionVersion  int
	IsActive        bool
	RevokedSessions int64
}

// CreateInactiveMerchantOwner creates a LOCAL MERCHANT user with is_active
// false. If the same email belongs to a MERCHANT and the password matches, it
// returns that user so a registration retry can resume.
func (r *Repository) CreateInactiveMerchantOwner(ctx context.Context, email, passwordHash, fullName, phone string) (User, bool, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return User{}, false, fmt.Errorf("creating merchant owner: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	user := User{}
	var role string
	var storedHash *string
	err = tx.QueryRow(ctx, `
		SELECT u.id, u.email, u.password_hash, u.full_name, u.phone, ro.name,
		       u.auth_provider, u.email_verified, u.session_version, u.is_active,
		       u.failed_login_count, u.locked_until, u.created_at
		FROM identity.users u
		JOIN identity.roles ro ON ro.id = u.role_id
		WHERE lower(u.email) = lower($1)
		FOR UPDATE OF u`,
		email,
	).Scan(&user.ID, &user.Email, &storedHash, &user.FullName, &user.Phone, &role,
		&user.AuthProvider, &user.EmailVerified, &user.SessionVersion, &user.IsActive,
		&user.FailedLoginCount, &user.LockedUntil, &user.CreatedAt)
	if err == nil {
		user.Role = auth.Role(role)
		user.PasswordHash = storedHash
		if user.Role != auth.RoleMerchant || storedHash == nil {
			return User{}, false, ErrEmailTaken
		}
		return user, false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return User{}, false, fmt.Errorf("creating merchant owner: %w", err)
	}

	user = User{
		Email:         email,
		PasswordHash:  &passwordHash,
		FullName:      fullName,
		Phone:         &phone,
		Role:          auth.RoleMerchant,
		AuthProvider:  "LOCAL",
		EmailVerified: false,
		IsActive:      false,
	}
	err = tx.QueryRow(ctx, `
		INSERT INTO identity.users (email, password_hash, full_name, phone, role_id, auth_provider, is_active)
		SELECT $1, $2, $3, $4, ro.id, 'LOCAL', false
		FROM identity.roles ro
		WHERE ro.name = 'MERCHANT'
		RETURNING id, session_version, created_at`,
		email, passwordHash, fullName, phone,
	).Scan(&user.ID, &user.SessionVersion, &user.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, false, errors.New("merchant role is not configured")
	}
	if isEmailTaken(err) {
		return User{}, false, ErrEmailTaken
	}
	if err != nil {
		return User{}, false, fmt.Errorf("creating merchant owner: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return User{}, false, fmt.Errorf("creating merchant owner: %w", err)
	}
	return user, true, nil
}

// ApplyMerchantLifecycleTransition applies one approval, rejection or
// suspension exactly once. It returns the transition state and current
// session authority state.
func (r *Repository) ApplyMerchantLifecycleTransition(
	ctx context.Context,
	userID, transitionID string,
	sequence int64,
	operation string,
	now time.Time,
) (MerchantLifecycleTransition, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return MerchantLifecycleTransition{}, fmt.Errorf("applying lifecycle transition: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var (
		role     string
		isActive bool
		version  int
	)
	err = tx.QueryRow(ctx, `
		SELECT ro.name, u.is_active, u.session_version
		FROM identity.users u
		JOIN identity.roles ro ON ro.id = u.role_id
		WHERE u.id = $1
		FOR UPDATE OF u`,
		userID,
	).Scan(&role, &isActive, &version)
	if errors.Is(err, pgx.ErrNoRows) {
		return MerchantLifecycleTransition{}, ErrNotFound
	}
	if err != nil {
		return MerchantLifecycleTransition{}, fmt.Errorf("applying lifecycle transition: %w", err)
	}
	if auth.Role(role) != auth.RoleMerchant {
		return MerchantLifecycleTransition{}, errors.New("user is not a merchant")
	}

	var existingOperation string
	var existingSequence int64
	var existingUserID string
	err = tx.QueryRow(ctx, `
		SELECT operation, sequence, user_id
		FROM identity.merchant_lifecycle_transitions
		WHERE transition_id = $1`,
		transitionID,
	).Scan(&existingOperation, &existingSequence, &existingUserID)
	if err == nil {
		if existingOperation != operation || existingSequence != sequence || existingUserID != userID {
			return MerchantLifecycleTransition{}, ErrTransitionConflict
		}
		if err := tx.Commit(ctx); err != nil {
			return MerchantLifecycleTransition{}, fmt.Errorf("applying lifecycle transition: %w", err)
		}
		return MerchantLifecycleTransition{
			TransitionID: transitionID, UserID: userID, Sequence: sequence,
			Operation: operation, AlreadyApplied: true, SessionVersion: version, IsActive: isActive,
		}, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return MerchantLifecycleTransition{}, fmt.Errorf("applying lifecycle transition: %w", err)
	}

	var lastSequence int64
	err = tx.QueryRow(ctx, `
		SELECT COALESCE(MAX(sequence), 0)
		FROM identity.merchant_lifecycle_transitions
		WHERE user_id = $1`,
		userID,
	).Scan(&lastSequence)
	if err != nil {
		return MerchantLifecycleTransition{}, fmt.Errorf("applying lifecycle transition: %w", err)
	}
	if sequence <= lastSequence {
		return MerchantLifecycleTransition{}, ErrTransitionStale
	}

	nextActive := operation == "APPROVE"
	var revoked int64
	if _, err := tx.Exec(ctx, `
		INSERT INTO identity.merchant_lifecycle_transitions (transition_id, user_id, sequence, operation, applied_at)
		VALUES ($1, $2, $3, $4, $5)`,
		transitionID, userID, sequence, operation, now.UTC(),
	); err != nil {
		return MerchantLifecycleTransition{}, fmt.Errorf("applying lifecycle transition: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE identity.users
		SET is_active = $2, session_version = session_version + 1
		WHERE id = $1`,
		userID, nextActive,
	); err != nil {
		return MerchantLifecycleTransition{}, fmt.Errorf("applying lifecycle transition: %w", err)
	}
	version++
	if !nextActive {
		tag, err := tx.Exec(ctx, `
			UPDATE identity.refresh_tokens
			SET revoked_at = $2
			WHERE user_id = $1 AND revoked_at IS NULL`,
			userID, now.UTC(),
		)
		if err != nil {
			return MerchantLifecycleTransition{}, fmt.Errorf("applying lifecycle transition: %w", err)
		}
		revoked = tag.RowsAffected()
	}
	if err := tx.Commit(ctx); err != nil {
		return MerchantLifecycleTransition{}, fmt.Errorf("applying lifecycle transition: %w", err)
	}
	return MerchantLifecycleTransition{
		TransitionID: transitionID, UserID: userID, Sequence: sequence,
		Operation: operation, AlreadyApplied: false, SessionVersion: version,
		IsActive: nextActive, RevokedSessions: revoked,
	}, nil
}

// SessionState returns the current role, active flag and session version.
func (r *Repository) SessionState(ctx context.Context, userID string) (SessionState, error) {
	var state SessionState
	var role string
	err := r.db.QueryRow(ctx, `
		SELECT ro.name, u.is_active, u.session_version
		FROM identity.users u
		JOIN identity.roles ro ON ro.id = u.role_id
		WHERE u.id = $1`,
		userID,
	).Scan(&role, &state.IsActive, &state.SessionVersion)
	if errors.Is(err, pgx.ErrNoRows) {
		return SessionState{}, ErrNotFound
	}
	if err != nil {
		return SessionState{}, fmt.Errorf("reading session state: %w", err)
	}
	state.Role = auth.Role(role)
	return state, nil
}

// DeletePendingMerchantOwner removes a never-provisioned inactive MERCHANT
// account. It refuses any account with sessions or lifecycle transitions.
func (r *Repository) DeletePendingMerchantOwner(ctx context.Context, userID string) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("deleting merchant owner: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var (
		role          string
		isActive      bool
		emailVerified bool
	)
	err = tx.QueryRow(ctx, `
		SELECT ro.name, u.is_active, u.email_verified
		FROM identity.users u
		JOIN identity.roles ro ON ro.id = u.role_id
		WHERE u.id = $1
		FOR UPDATE OF u`,
		userID,
	).Scan(&role, &isActive, &emailVerified)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("deleting merchant owner: %w", err)
	}
	if auth.Role(role) != auth.RoleMerchant || isActive || emailVerified {
		return errors.New("merchant owner is not eligible for compensation")
	}
	var sessions int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM identity.refresh_tokens WHERE user_id = $1`, userID).Scan(&sessions); err != nil {
		return fmt.Errorf("deleting merchant owner: %w", err)
	}
	var transitions int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM identity.merchant_lifecycle_transitions WHERE user_id = $1`, userID).Scan(&transitions); err != nil {
		return fmt.Errorf("deleting merchant owner: %w", err)
	}
	if sessions > 0 || transitions > 0 {
		return errors.New("merchant owner is not eligible for compensation")
	}
	if _, err := tx.Exec(ctx, `DELETE FROM identity.users WHERE id = $1`, userID); err != nil {
		return fmt.Errorf("deleting merchant owner: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("deleting merchant owner: %w", err)
	}
	return nil
}
