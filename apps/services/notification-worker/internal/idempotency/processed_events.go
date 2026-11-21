package idempotency

import (
	"context"
	"errors"
	"fmt"
	"time"
)

const deleteProcessedEventsBeforeSQL = `
	DELETE FROM notification.processed_events
	WHERE processed_at < $1`

// DeleteProcessedEventsBefore deletes processed-event records strictly older
// than cutoff and returns the number of affected rows. It never touches
// notification delivery logs or changes idempotency behavior for retained rows.
func (p *Postgres) DeleteProcessedEventsBefore(ctx context.Context, cutoff time.Time) (int64, error) {
	if p == nil || p.db == nil {
		return 0, ErrUnavailable
	}
	if err := ctx.Err(); err != nil {
		return 0, fmt.Errorf("%w: %w", ErrCleanupFailed, err)
	}

	tag, err := p.db.Exec(ctx, deleteProcessedEventsBeforeSQL, cutoff.UTC())
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return 0, fmt.Errorf("%w: %w", ErrCleanupFailed, ctxErr)
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return 0, fmt.Errorf("%w: %w", ErrCleanupFailed, err)
		}
		return 0, fmt.Errorf("%w: delete", ErrCleanupFailed)
	}
	return tag.RowsAffected(), nil
}
