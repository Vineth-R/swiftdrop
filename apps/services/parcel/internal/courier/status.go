package courier

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Vineth-R/swiftdrop/apps/services/parcel/internal/repository"
	"github.com/Vineth-R/swiftdrop/apps/services/parcel/internal/statemachine"
)

// StatusInput carries the authenticated courier separately from the request fields.
type StatusInput struct {
	ParcelID, CourierID string
	Status              statemachine.ParcelStatus
	RecipientName       string
	FailureReason       string
	Note                string
}

type StatusResult struct {
	ParcelID       string                    `json:"parcelId"`
	TrackingNumber string                    `json:"trackingNumber"`
	PreviousStatus statemachine.ParcelStatus `json:"previousStatus"`
	Status         statemachine.ParcelStatus `json:"status"`
	UpdatedAt      time.Time                 `json:"updatedAt"`
}

// UpdateStatus serializes courier writes with M6's assignment transaction by
// locking the parcel first. No broker or consumer participates in this transaction.
func (r *Repository) UpdateStatus(ctx context.Context, input StatusInput) (result StatusResult, err error) {
	if r.db == nil {
		return StatusResult{}, ErrUnavailable
	}
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return StatusResult{}, fmt.Errorf("begin courier status update: %w", err)
	}
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if rollbackErr := tx.Rollback(cleanupCtx); rollbackErr != nil && !errors.Is(rollbackErr, pgx.ErrTxClosed) {
			err = errors.Join(err, fmt.Errorf("rollback courier status update: %w", rollbackErr))
		}
	}()

	result, err = updateStatusInTx(ctx, tx, input)
	if err != nil {
		return StatusResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return StatusResult{}, fmt.Errorf("commit courier status update: %w", err)
	}
	return result, nil
}

func updateStatusInTx(ctx context.Context, tx pgx.Tx, input StatusInput) (StatusResult, error) {
	result := StatusResult{ParcelID: input.ParcelID, Status: input.Status}
	var currentCourierID, customerID, merchantID *string
	err := tx.QueryRow(ctx, `SELECT tracking_number, status, current_courier_id,
		customer_user_id, merchant_id FROM parcel.parcels WHERE id = $1 FOR UPDATE`, input.ParcelID).
		Scan(&result.TrackingNumber, &result.PreviousStatus, &currentCourierID, &customerID, &merchantID)
	if errors.Is(err, pgx.ErrNoRows) {
		return StatusResult{}, ErrNotFound
	}
	if err != nil {
		return StatusResult{}, fmt.Errorf("lock courier parcel: %w", err)
	}
	owner := ""
	if currentCourierID != nil {
		owner = *currentCourierID
	}
	if err := statemachine.ValidateCourierTransition(owner, input.CourierID,
		result.PreviousStatus, input.Status, statemachine.TransitionInput{
			RecipientName: input.RecipientName, FailureReason: input.FailureReason, Note: input.Note,
		}); err != nil {
		return StatusResult{}, err
	}

	var recipient, reason, note *string
	if input.Status == statemachine.StatusDelivered {
		value := strings.TrimSpace(input.RecipientName)
		recipient = &value
	}
	if input.Status == statemachine.StatusFailed {
		value := strings.TrimSpace(input.FailureReason)
		reason = &value
	}
	if input.Note != "" {
		note = &input.Note
	}
	err = tx.QueryRow(ctx, `UPDATE parcel.parcels SET status = $2::text,
		failure_reason = $3, delivered_to_name = $4,
		delivered_at = CASE WHEN $2 = 'DELIVERED' THEN now() ELSE delivered_at END,
		failed_at = CASE WHEN $2 = 'FAILED' THEN now() ELSE failed_at END,
		updated_at = now() WHERE id = $1 RETURNING updated_at`,
		input.ParcelID, input.Status, reason, recipient).Scan(&result.UpdatedAt)
	if err != nil {
		return StatusResult{}, fmt.Errorf("update courier parcel status: %w", err)
	}

	historyID, err := statusHistoryID()
	if err != nil {
		return StatusResult{}, err
	}
	parcelRepo, err := repository.New(tx)
	if err != nil {
		return StatusResult{}, fmt.Errorf("create courier history writer: %w", err)
	}
	previous := repository.ParcelStatus(result.PreviousStatus)
	role := "COURIER"
	_, err = parcelRepo.AppendStatusHistory(ctx, repository.AppendStatusHistoryParams{
		ID: historyID, ParcelID: input.ParcelID, PreviousStatus: &previous,
		NewStatus: repository.ParcelStatus(input.Status), ActorUserID: &input.CourierID,
		ActorRole: &role, Note: note, FailureReason: reason,
	})
	if err != nil {
		return StatusResult{}, fmt.Errorf("append courier status history: %w", err)
	}

	// Store only the canonical data object. M6's relay supplies the envelope and
	// maps these legacy storage labels to parcel.status_changed.v1, version 1.
	payload := map[string]any{
		"parcel_id": input.ParcelID, "tracking_number": result.TrackingNumber,
		"previous_status": result.PreviousStatus, "status": input.Status,
		"actor_user_id": input.CourierID,
	}
	if customerID != nil {
		payload["customer_user_id"] = *customerID
	}
	if merchantID != nil {
		payload["merchant_id"] = *merchantID
	}
	if note != nil {
		payload["note"] = *note
	}
	if reason != nil {
		payload["failure_reason"] = *reason
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return StatusResult{}, fmt.Errorf("encode courier status event: %w", err)
	}
	_, err = tx.Exec(ctx, `INSERT INTO parcel.outbox_events
		(id, event_type, routing_key, payload, status, attempts, aggregate_type, aggregate_id)
		VALUES (gen_random_uuid(), 'parcel.status_changed', 'parcel.status.changed',
		$1, 'PENDING', 0, 'parcel', $2)`, data, input.ParcelID)
	if err != nil {
		return StatusResult{}, fmt.Errorf("insert courier status event: %w", err)
	}
	result.UpdatedAt = result.UpdatedAt.UTC()
	return result, nil
}

func statusHistoryID() (string, error) {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return "", fmt.Errorf("generate courier history ID: %w", err)
	}
	id[6] = (id[6] & 0x0f) | 0x40
	id[8] = (id[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", id[:4], id[4:6], id[6:8], id[8:10], id[10:]), nil
}
