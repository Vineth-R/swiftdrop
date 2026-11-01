package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

type StatusHistory struct {
	ID             string
	ParcelID       string
	PreviousStatus *ParcelStatus
	NewStatus      ParcelStatus
	ActorUserID    *string
	ActorRole      *string
	Note           *string
	FailureReason  *string
	CreatedAt      time.Time
}

type AppendStatusHistoryParams struct {
	ID             string
	ParcelID       string
	PreviousStatus *ParcelStatus
	NewStatus      ParcelStatus
	ActorUserID    *string
	ActorRole      *string
	Note           *string
	FailureReason  *string
}

const historyColumns = `id, parcel_id, previous_status, new_status, actor_user_id,
	actor_role, note, failure_reason, created_at`

// AppendStatusHistory must share the transaction that creates or updates the
// parcel and inserts its outbox event. There are deliberately no history update
// or delete methods: the database enforces append-only history.
func (r *Repository) AppendStatusHistory(
	ctx context.Context,
	params AppendStatusHistoryParams,
) (StatusHistory, error) {
	history, err := scanStatusHistory(r.db.QueryRow(
		ctx,
		`INSERT INTO parcel.parcel_status_history (
			id, parcel_id, previous_status, new_status, actor_user_id,
			actor_role, note, failure_reason
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING `+historyColumns,
		params.ID,
		params.ParcelID,
		params.PreviousStatus,
		params.NewStatus,
		params.ActorUserID,
		params.ActorRole,
		params.Note,
		params.FailureReason,
	))
	if err != nil {
		return StatusHistory{}, fmt.Errorf("appending parcel status history: %w", databaseError(err))
	}

	return history, nil
}

func (r *Repository) StatusHistoryByParcel(ctx context.Context, parcelID string) ([]StatusHistory, error) {
	rows, err := r.db.Query(
		ctx,
		`SELECT `+historyColumns+` FROM parcel.parcel_status_history
		 WHERE parcel_id = $1 ORDER BY created_at, id`,
		parcelID,
	)
	if err != nil {
		return nil, fmt.Errorf("querying parcel status history: %w", databaseError(err))
	}
	defer rows.Close()

	history := make([]StatusHistory, 0)
	for rows.Next() {
		entry, err := scanStatusHistory(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning parcel status history: %w", databaseError(err))
		}
		history = append(history, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating parcel status history: %w", databaseError(err))
	}

	return history, nil
}

func scanStatusHistory(row pgx.Row) (StatusHistory, error) {
	var history StatusHistory
	err := row.Scan(
		&history.ID,
		&history.ParcelID,
		&history.PreviousStatus,
		&history.NewStatus,
		&history.ActorUserID,
		&history.ActorRole,
		&history.Note,
		&history.FailureReason,
		&history.CreatedAt,
	)
	if err != nil {
		return StatusHistory{}, err
	}

	return history, nil
}
