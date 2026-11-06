// Package retention runs periodic cleanup of expired notification idempotency
// records without participating in notification delivery.
package retention

import (
	"context"
	"log/slog"
	"time"
)

// Cleaner deletes processed-event records older than cutoff.
type Cleaner interface {
	DeleteProcessedEventsBefore(ctx context.Context, cutoff time.Time) (int64, error)
}

// Run cleans immediately and then on every interval until ctx is canceled.
// Cleanup runs serially in this goroutine, so runs cannot overlap.
func Run(
	ctx context.Context,
	logger *slog.Logger,
	cleaner Cleaner,
	retention time.Duration,
	interval time.Duration,
) {
	if logger == nil {
		logger = slog.Default()
	}
	if cleaner == nil || retention <= 0 || interval <= 0 {
		logger.ErrorContext(ctx, "processed-event cleanup disabled", "operation", "cleanup_processed_events")
		return
	}
	if ctx.Err() != nil {
		return
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	run(ctx, logger, cleaner, retention, time.Now, ticker.C)
}

func run(
	ctx context.Context,
	logger *slog.Logger,
	cleaner Cleaner,
	retention time.Duration,
	now func() time.Time,
	ticks <-chan time.Time,
) {
	if ctx.Err() != nil {
		return
	}

	cleanup(ctx, logger, cleaner, retention, now)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticks:
			cleanup(ctx, logger, cleaner, retention, now)
		}
	}
}

func cleanup(
	ctx context.Context,
	logger *slog.Logger,
	cleaner Cleaner,
	retention time.Duration,
	now func() time.Time,
) {
	cutoff := now().UTC().Add(-retention)
	deleted, err := cleaner.DeleteProcessedEventsBefore(ctx, cutoff)
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		logger.ErrorContext(ctx, "processed-event cleanup failed", "operation", "cleanup_processed_events")
		return
	}
	logger.InfoContext(ctx, "processed-event cleanup completed", "deleted", deleted)
}
