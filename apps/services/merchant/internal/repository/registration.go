package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/businessreg"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const (
	uniqueViolation  = "23505"
	idempotencyIndex = "merchant_registration_active_email_uq"
	businessRegIndex = "merchants_business_reg_no_uq"
	ownerIndex       = "merchants_owner_user_id_key"
)

// RegistrationAttempt is the persisted state of one merchant registration saga.
type RegistrationAttempt struct {
	ID             string
	IdempotencyKey string
	Email          string
	Status         string
	UserID         *string
	MerchantID     *string
	LastError      *string
}

// EnsureRegistrationAttempt inserts a PENDING attempt or returns the existing
// attempt for the same idempotency key. A different key for an active or
// completed application is rejected.
func (r *Repository) EnsureRegistrationAttempt(ctx context.Context, id, idempotencyKey, email string) (RegistrationAttempt, error) {
	attempt, err := r.registrationAttempt(ctx, "idempotency_key = $1", idempotencyKey)
	if err == nil {
		return attempt, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return RegistrationAttempt{}, err
	}
	_, err = r.db.Exec(ctx, `
		INSERT INTO merchant.merchant_registration_attempts (id, idempotency_key, email)
		VALUES ($1, $2, $3)`, id, idempotencyKey, email)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
			if attempt, lookupErr := r.registrationAttempt(ctx, "idempotency_key = $1", idempotencyKey); lookupErr == nil {
				return attempt, nil
			}
			if pgErr.ConstraintName == idempotencyIndex {
				return RegistrationAttempt{}, ErrDuplicateApplication
			}
			return RegistrationAttempt{}, ErrRegistrationConflict
		}
		return RegistrationAttempt{}, fmt.Errorf("creating registration attempt: %w", err)
	}
	return r.registrationAttempt(ctx, "id = $1", id)
}

func (r *Repository) registrationAttempt(ctx context.Context, condition string, arg any) (RegistrationAttempt, error) {
	attempt := RegistrationAttempt{}
	err := r.db.QueryRow(ctx, `
		SELECT id, idempotency_key, email, status, user_id, merchant_id, last_error
		FROM merchant.merchant_registration_attempts
		WHERE `+condition, arg,
	).Scan(&attempt.ID, &attempt.IdempotencyKey, &attempt.Email, &attempt.Status,
		&attempt.UserID, &attempt.MerchantID, &attempt.LastError)
	if errors.Is(err, pgx.ErrNoRows) {
		return RegistrationAttempt{}, ErrNotFound
	}
	if err != nil {
		return RegistrationAttempt{}, fmt.Errorf("reading registration attempt: %w", err)
	}
	return attempt, nil
}

func (r *Repository) SetRegistrationAttemptUser(ctx context.Context, attemptID, userID string) error {
	_, err := r.db.Exec(ctx, `UPDATE merchant.merchant_registration_attempts SET user_id = $2, status = 'PENDING', last_error = NULL WHERE id = $1`, attemptID, userID)
	if err != nil {
		return fmt.Errorf("updating registration attempt: %w", err)
	}
	return nil
}

func (r *Repository) CompleteRegistrationAttempt(ctx context.Context, attemptID, merchantID string) error {
	_, err := r.db.Exec(ctx, `UPDATE merchant.merchant_registration_attempts SET status = 'COMPLETED', merchant_id = $2, last_error = NULL WHERE id = $1`, attemptID, merchantID)
	if err != nil {
		return fmt.Errorf("completing registration attempt: %w", err)
	}
	return nil
}

func (r *Repository) FailRegistrationAttempt(ctx context.Context, attemptID, message string) error {
	_, err := r.db.Exec(ctx, `UPDATE merchant.merchant_registration_attempts SET status = 'FAILED', last_error = $2 WHERE id = $1`, attemptID, message)
	if err != nil {
		return fmt.Errorf("failing registration attempt: %w", err)
	}
	return nil
}

// CreatePendingMerchant inserts a PENDING merchant and completes the attempt in
// one transaction.
func (r *Repository) CreatePendingMerchant(ctx context.Context, attemptID string, params CreateMerchantParams) (Merchant, error) {
	businessRegNo, err := businessreg.NormalizeValid(params.BusinessRegNo)
	if err != nil {
		return Merchant{}, ErrInvalidRegistrationNumber
	}
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Merchant{}, fmt.Errorf("creating merchant: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	_, err = tx.Exec(ctx, `
		INSERT INTO merchant.merchants (
			id, owner_user_id, business_name, business_reg_no, business_address, contact_email,
			contact_phone, pickup_address, pickup_zone_id
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		params.ID, params.OwnerUserID, params.BusinessName, businessRegNo, nullable(params.BusinessAddress),
		params.ContactEmail, params.ContactPhone, params.PickupAddress, params.PickupZoneID)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
			switch pgErr.ConstraintName {
			case ownerIndex:
				return Merchant{}, ErrDuplicateApplication
			case businessRegIndex:
				return Merchant{}, ErrDuplicateBusiness
			default:
				return Merchant{}, ErrRegistrationConflict
			}
		}
		return Merchant{}, fmt.Errorf("creating merchant: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE merchant.merchant_registration_attempts SET status = 'COMPLETED', merchant_id = $2, last_error = NULL WHERE id = $1`, attemptID, params.ID); err != nil {
		return Merchant{}, fmt.Errorf("completing registration attempt: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Merchant{}, fmt.Errorf("creating merchant: %w", err)
	}
	return r.MerchantByID(ctx, params.ID)
}
