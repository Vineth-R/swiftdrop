// Package retention runs periodic cleanup jobs for the notification worker
// without participating in notification delivery. The processed-event and
// notification-log policies remain independent.
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

// CleanerFunc deletes rows older than cutoff and returns the deleted count.
type CleanerFunc func(ctx context.Context, cutoff time.Time) (int64, error)

// Job describes one independently configured cleanup schedule.
type Job struct {
	Name        string
	Operation   string
	Retention   time.Duration
	Interval    time.Duration
	Clean       CleanerFunc
	LogDuration bool
	LogError    bool
}

// Run cleans processed events immediately and then on every interval until ctx
// is canceled. Cleanup runs serially in this goroutine, so runs cannot overlap.
func Run(
	ctx context.Context,
	logger *slog.Logger,
	cleaner Cleaner,
	retention time.Duration,
	interval time.Duration,
) {
	RunJob(ctx, logger, processedEventJob(cleaner, retention, interval))
}

// RunJob cleans immediately and then on every interval until ctx is canceled.
// Cleanup runs serially in this goroutine, so runs cannot overlap. A failed run
// is retried on the next tick.
func RunJob(ctx context.Context, logger *slog.Logger, job Job) {
	if logger == nil {
		logger = slog.Default()
	}
	if job.Clean == nil || job.Retention <= 0 || job.Interval <= 0 {
		logger.ErrorContext(ctx, job.Name+" cleanup disabled", "operation", job.Operation)
		return
	}
	if ctx.Err() != nil {
		return
	}

	ticker := time.NewTicker(job.Interval)
	defer ticker.Stop()
	runJob(ctx, logger, job, time.Now, ticker.C)
}

func processedEventJob(cleaner Cleaner, retention, interval time.Duration) Job {
	var clean CleanerFunc
	if cleaner != nil {
		clean = cleaner.DeleteProcessedEventsBefore
	}
	return Job{
		Name:      "processed-event",
		Operation: "cleanup_processed_events",
		Retention: retention,
		Interval:  interval,
		Clean:     clean,
	}
}

func run(
	ctx context.Context,
	logger *slog.Logger,
	cleaner Cleaner,
	retention time.Duration,
	now func() time.Time,
	ticks <-chan time.Time,
) {
	runJob(ctx, logger, processedEventJob(cleaner, retention, time.Hour), now, ticks)
}

func runJob(
	ctx context.Context,
	logger *slog.Logger,
	job Job,
	now func() time.Time,
	ticks <-chan time.Time,
) {
	if ctx.Err() != nil {
		return
	}

	cleanupJob(ctx, logger, job, now)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticks:
			cleanupJob(ctx, logger, job, now)
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
	cleanupJob(ctx, logger, processedEventJob(cleaner, retention, time.Hour), now)
}

func cleanupJob(
	ctx context.Context,
	logger *slog.Logger,
	job Job,
	now func() time.Time,
) {
	cutoff := now().UTC().Add(-job.Retention)
	started := time.Now()
	deleted, err := job.Clean(ctx, cutoff)
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		attributes := []any{"operation", job.Operation}
		if job.LogError {
			attributes = append(attributes, "error", err)
		}
		logger.ErrorContext(ctx, job.Name+" cleanup failed", attributes...)
		return
	}
	attributes := []any{"deleted", deleted}
	if job.LogDuration {
		attributes = append(attributes, "duration_ms", time.Since(started).Milliseconds())
	}
	logger.InfoContext(ctx, job.Name+" cleanup completed", attributes...)
}
