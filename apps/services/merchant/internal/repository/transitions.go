package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// StatusTransition is one merchant lifecycle change and its identity sync state.
type StatusTransition struct {
	ID          string
	MerchantID  string
	Sequence    int64
	FromStatus  MerchantStatus
	ToStatus    MerchantStatus
	ActorUserID string
	Reason      *string
	SyncStatus  string
	Attempts    int
	LastError   *string
	CreatedAt   time.Time
	AppliedAt   *time.Time
}

// LatestTransition returns the highest-sequence transition for a merchant,
// optionally filtered by target status.
func (r *Repository) LatestTransition(ctx context.Context, merchantID string, target MerchantStatus) (StatusTransition, error) {
	condition := "merchant_id = $1"
	args := []any{merchantID}
	if target != "" {
		condition += " AND to_status = $2"
		args = append(args, string(target))
	}
	transition := StatusTransition{}
	var reason, lastError *string
	err := r.db.QueryRow(ctx, `
		SELECT id, merchant_id, sequence, from_status, to_status, actor_user_id,
		       reason, sync_status, attempts, last_error, created_at, applied_at
		FROM merchant.merchant_status_transitions
		WHERE `+condition+`
		ORDER BY sequence DESC LIMIT 1`, args...,
	).Scan(&transition.ID, &transition.MerchantID, &transition.Sequence,
		&transition.FromStatus, &transition.ToStatus, &transition.ActorUserID,
		&reason, &transition.SyncStatus, &transition.Attempts, &lastError,
		&transition.CreatedAt, &transition.AppliedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return StatusTransition{}, ErrNotFound
	}
	if err != nil {
		return StatusTransition{}, fmt.Errorf("reading status transition: %w", err)
	}
	transition.Reason, transition.LastError = reason, lastError
	return transition, nil
}

// BeginApprovalTransition allocates the next sequence and records a PENDING
// approval transition without changing the merchant status.
func (r *Repository) BeginApprovalTransition(ctx context.Context, merchantID, actorID, transitionID string) (Merchant, StatusTransition, error) {
	return r.newTransition(ctx, merchantID, actorID, transitionID, MerchantStatusApproved, nil, false)
}

// BeginRejectionTransition changes the merchant to REJECTED immediately and
// records a PENDING identity transition with the optional merchant-facing
// reason.
func (r *Repository) BeginRejectionTransition(ctx context.Context, merchantID, actorID, transitionID string, reason *string) (Merchant, StatusTransition, error) {
	return r.newTransition(ctx, merchantID, actorID, transitionID, MerchantStatusRejected, reason, true)
}

// BeginSuspensionTransition changes the merchant to SUSPENDED immediately and
// records a PENDING identity transition.
func (r *Repository) BeginSuspensionTransition(ctx context.Context, merchantID, actorID, transitionID string, reason *string) (Merchant, StatusTransition, error) {
	return r.newTransition(ctx, merchantID, actorID, transitionID, MerchantStatusSuspended, reason, true)
}

func (r *Repository) newTransition(ctx context.Context, merchantID, actorID, transitionID string, target MerchantStatus, reason *string, applyStatus bool) (Merchant, StatusTransition, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Merchant{}, StatusTransition{}, fmt.Errorf("begin merchant transition: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	current := MerchantStatus("")
	var sequence int64
	if err := tx.QueryRow(ctx, `SELECT status, lifecycle_sequence FROM merchant.merchants WHERE id = $1 FOR UPDATE`, merchantID).Scan(&current, &sequence); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Merchant{}, StatusTransition{}, ErrNotFound
		}
		return Merchant{}, StatusTransition{}, fmt.Errorf("lock merchant: %w", err)
	}
	if !validTransition(current, target) {
		return Merchant{}, StatusTransition{}, fmt.Errorf("%w from %s to %s", ErrInvalidState, current, target)
	}
	sequence++
	if _, err := tx.Exec(ctx, `UPDATE merchant.merchants SET lifecycle_sequence = $2 WHERE id = $1`, merchantID, sequence); err != nil {
		return Merchant{}, StatusTransition{}, fmt.Errorf("allocating lifecycle sequence: %w", err)
	}
	if applyStatus {
		action := "REJECTED"
		if target == MerchantStatusSuspended {
			action = "SUSPENDED"
		}
		reasonValue := ""
		if reason != nil {
			reasonValue = *reason
		}
		details := map[string]any{
			"transitionId":   transitionID,
			"sequence":       sequence,
			"merchantReason": nullable(reasonValue),
			"reasonProvided": reason != nil && reasonValue != "",
		}
		if target == MerchantStatusRejected {
			_, err = tx.Exec(ctx, `UPDATE merchant.merchants SET status = $1, rejection_reason = $2, updated_at = now() WHERE id = $3`, target, nullable(reasonValue), merchantID)
		} else {
			_, err = tx.Exec(ctx, `UPDATE merchant.merchants SET status = $1, suspension_reason = $2, updated_at = now() WHERE id = $3`, target, nullable(reasonValue), merchantID)
		}
		if err != nil {
			return Merchant{}, StatusTransition{}, fmt.Errorf("update merchant state: %w", err)
		}
		if err := insertMerchantAudit(ctx, tx, actorID, action, merchantID, details); err != nil {
			return Merchant{}, StatusTransition{}, err
		}
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO merchant.merchant_status_transitions
			(id, merchant_id, sequence, from_status, to_status, actor_user_id, reason, sync_status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, 'PENDING')`,
		transitionID, merchantID, sequence, string(current), string(target), actorID, nullString(reason),
	); err != nil {
		return Merchant{}, StatusTransition{}, fmt.Errorf("record merchant transition: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Merchant{}, StatusTransition{}, fmt.Errorf("commit merchant transition: %w", err)
	}
	merchant, err := r.MerchantByID(ctx, merchantID)
	if err != nil {
		return Merchant{}, StatusTransition{}, err
	}
	transition, err := r.LatestTransition(ctx, merchantID, target)
	if err != nil {
		return Merchant{}, StatusTransition{}, err
	}
	return merchant, transition, nil
}

// ApplyApprovalTransition commits APPROVED after Identity has applied the
// lifecycle transition.
func (r *Repository) ApplyApprovalTransition(ctx context.Context, merchantID, actorID, transitionID string) (Merchant, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Merchant{}, fmt.Errorf("apply approval: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var syncStatus string
	err = tx.QueryRow(ctx, `SELECT sync_status FROM merchant.merchant_status_transitions WHERE id = $1 AND merchant_id = $2 FOR UPDATE`, transitionID, merchantID).Scan(&syncStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		return Merchant{}, ErrNotFound
	}
	if err != nil {
		return Merchant{}, fmt.Errorf("apply approval: %w", err)
	}
	if syncStatus == "APPLIED" {
		if err := tx.Commit(ctx); err != nil {
			return Merchant{}, fmt.Errorf("apply approval: %w", err)
		}
		return r.MerchantByID(ctx, merchantID)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE merchant.merchants
		SET status = 'APPROVED', approved_by = $2, approved_at = now(),
		    rejection_reason = NULL, suspension_reason = NULL, updated_at = now()
		WHERE id = $1`, merchantID, actorID); err != nil {
		return Merchant{}, fmt.Errorf("apply approval: %w", err)
	}
	if err := insertMerchantAudit(ctx, tx, actorID, "APPROVED", merchantID, map[string]any{"transitionId": transitionID}); err != nil {
		return Merchant{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE merchant.merchant_status_transitions SET sync_status = 'APPLIED', applied_at = now(), last_error = NULL WHERE id = $1`, transitionID); err != nil {
		return Merchant{}, fmt.Errorf("apply approval: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Merchant{}, fmt.Errorf("apply approval: %w", err)
	}
	return r.MerchantByID(ctx, merchantID)
}

// MarkTransitionResult records the identity synchronization outcome.
func (r *Repository) MarkTransitionResult(ctx context.Context, transitionID string, applied bool, message string) error {
	status := "FAILED"
	var appliedAt any
	if applied {
		status = "APPLIED"
		appliedAt = time.Now().UTC()
	}
	_, err := r.db.Exec(ctx, `
		UPDATE merchant.merchant_status_transitions
		SET sync_status = $2, applied_at = $3, attempts = attempts + 1,
		    last_error = NULLIF($4, '')
		WHERE id = $1`, transitionID, status, appliedAt, message)
	if err != nil {
		return fmt.Errorf("mark transition: %w", err)
	}
	return nil
}

func nullString(value *string) any {
	if value == nil || *value == "" {
		return nil
	}
	return *value
}

func insertMerchantAudit(ctx context.Context, tx pgx.Tx, actorID, action, merchantID string, details map[string]any) error {
	payload, err := json.Marshal(details)
	if err != nil {
		return fmt.Errorf("record merchant audit: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO merchant.audit_logs (id, actor_user_id, action, entity_type, entity_id, details)
		VALUES (gen_random_uuid(), $1, $2, 'merchant', $3, $4::jsonb)`,
		actorID, action, merchantID, payload,
	); err != nil {
		return fmt.Errorf("record merchant audit: %w", err)
	}
	return nil
}
