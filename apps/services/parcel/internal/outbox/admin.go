package outbox

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// FailedSummary reports how many outbox events exhausted their publish
// attempts and how old the oldest one is. Operators use it to decide whether to
// replay or expire failed events.
type FailedSummary struct {
	Count  int64      `json:"count"`
	Oldest *time.Time `json:"oldest,omitempty"`
}

// FailedSummary reads the failed-event backlog. It is a read-only operation.
func (s *SQLStore) FailedSummary(ctx context.Context) (FailedSummary, error) {
	if s == nil || s.pool == nil {
		return FailedSummary{}, errors.New("database pool is not configured")
	}
	var summary FailedSummary
	if err := s.pool.QueryRow(ctx, `
		SELECT count(*), min(created_at)
		FROM parcel.outbox_events
		WHERE status = 'FAILED'`,
	).Scan(&summary.Count, &summary.Oldest); err != nil {
		return FailedSummary{}, fmt.Errorf("summarise failed outbox events: %w", err)
	}
	return summary, nil
}

// ReplayFailed returns failed events older than cutoff to PENDING with a fresh
// attempt budget. The claim fields are cleared so any relay replica may take
// them. Replaying is idempotent with respect to the relay: an event that is
// already PUBLISHED or PENDING is not touched.
func (s *SQLStore) ReplayFailed(ctx context.Context, cutoff time.Time, limit int) (int64, error) {
	if s == nil || s.pool == nil {
		return 0, errors.New("database pool is not configured")
	}
	if limit <= 0 {
		limit = 100
	}
	if cutoff.IsZero() {
		cutoff = time.Now().UTC()
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE parcel.outbox_events AS e
		SET status = 'PENDING',
		    attempts = 0,
		    claimed_at = NULL,
		    claim_token = NULL
		WHERE e.id IN (
			SELECT id
			FROM parcel.outbox_events
			WHERE status = 'FAILED' AND created_at < $1
			ORDER BY created_at ASC
			LIMIT $2
			FOR UPDATE SKIP LOCKED
		)`,
		cutoff.UTC(), limit,
	)
	if err != nil {
		return 0, fmt.Errorf("replay failed outbox events: %w", err)
	}
	return tag.RowsAffected(), nil
}

// DeleteFailed expires failed events older than cutoff. It never touches
// PENDING or PUBLISHED rows. The caller must make the retention decision
// explicitly; this method is destructive by design.
func (s *SQLStore) DeleteFailed(ctx context.Context, cutoff time.Time, limit int) (int64, error) {
	if s == nil || s.pool == nil {
		return 0, errors.New("database pool is not configured")
	}
	if limit <= 0 {
		limit = 100
	}
	if cutoff.IsZero() {
		return 0, errors.New("failed-event retention cutoff is required")
	}
	tag, err := s.pool.Exec(ctx, `
		DELETE FROM parcel.outbox_events
		WHERE id IN (
			SELECT id
			FROM parcel.outbox_events
			WHERE status = 'FAILED' AND created_at < $1
			ORDER BY created_at ASC
			LIMIT $2
		)`,
		cutoff.UTC(), limit,
	)
	if err != nil {
		return 0, fmt.Errorf("delete failed outbox events: %w", err)
	}
	return tag.RowsAffected(), nil
}
