package courier

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Vineth-R/swiftdrop/apps/services/parcel/internal/statemachine"
)

// statusEvent is the canonical data object stored by the parcel outbox. The
// relay supplies the envelope and maps the storage label to the versioned
// parcel.status_changed.v1 routing key.
type statusEvent struct {
	ParcelID       string
	TrackingNumber string
	PreviousStatus statemachine.ParcelStatus
	Status         statemachine.ParcelStatus
	ActorUserID    string
	CustomerID     *string
	MerchantID     *string
	Note           *string
	FailureReason  *string
	OccurredAt     time.Time
}

func writeStatusEvent(ctx context.Context, tx pgx.Tx, event statusEvent) error {
	payload := map[string]any{
		"parcel_id": event.ParcelID, "tracking_number": event.TrackingNumber,
		"previous_status": event.PreviousStatus, "status": event.Status,
		"actor_user_id": event.ActorUserID,
	}
	if event.CustomerID != nil {
		payload["customer_user_id"] = *event.CustomerID
	}
	if event.MerchantID != nil {
		payload["merchant_id"] = *event.MerchantID
	}
	if event.Note != nil {
		payload["note"] = *event.Note
	}
	if event.FailureReason != nil {
		payload["failure_reason"] = *event.FailureReason
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode courier status event: %w", err)
	}
	_, err = tx.Exec(ctx, `INSERT INTO parcel.outbox_events
		(id, event_type, routing_key, payload, status, attempts, aggregate_type, aggregate_id, created_at)
		VALUES (gen_random_uuid(), 'parcel.status_changed', 'parcel.status.changed',
		$1, 'PENDING', 0, 'parcel', $2, $3)`, data, event.ParcelID, event.OccurredAt)
	if err != nil {
		return fmt.Errorf("insert courier status event: %w", err)
	}
	return nil
}
