package retention

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func notificationLogJob(clean CleanerFunc, retention time.Duration) Job {
	return Job{
		Name:        "notification-log",
		Operation:   "cleanup_notification_logs",
		Retention:   retention,
		Interval:    time.Hour,
		Clean:       clean,
		LogDuration: true,
		LogError:    true,
	}
}

func TestRunJobCleansImmediatelyWithUTCcutoff(t *testing.T) {
	fixedNow := time.Date(2026, time.November, 16, 12, 0, 0, 0, time.UTC)
	retention := 180 * 24 * time.Hour
	cleaner := newFakeCleaner()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	done := make(chan struct{})
	go func() {
		defer close(done)
		runJob(ctx, discardLogger(), notificationLogJob(cleaner.DeleteProcessedEventsBefore, retention), func() time.Time { return fixedNow }, make(chan time.Time))
	}()

	call := waitForCall(t, cleaner.calls)
	if !call.cutoff.Equal(fixedNow.Add(-retention)) {
		t.Fatalf("cutoff = %v, want %v", call.cutoff, fixedNow.Add(-retention))
	}
	if call.cutoff.Location() != time.UTC {
		t.Fatalf("cutoff location = %v, want UTC", call.cutoff.Location())
	}
	cancel()
	waitForDone(t, done)
}

func TestRunJobRetriesAfterFailureWithoutStopping(t *testing.T) {
	fixedNow := time.Date(2026, time.November, 16, 12, 0, 0, 0, time.UTC)
	cleaner := newFakeCleaner()
	cleaner.failFirst = true
	ticks := make(chan time.Time, 1)
	var buffer bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buffer, nil))
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	done := make(chan struct{})
	go func() {
		defer close(done)
		runJob(ctx, logger, notificationLogJob(cleaner.DeleteProcessedEventsBefore, 180*24*time.Hour), func() time.Time { return fixedNow }, ticks)
	}()

	waitForCall(t, cleaner.calls)
	ticks <- fixedNow
	waitForCall(t, cleaner.calls)
	if cleaner.callCount() != 2 {
		t.Fatalf("cleanup calls = %d, want 2", cleaner.callCount())
	}
	cancel()
	waitForDone(t, done)
	if !strings.Contains(buffer.String(), "notification-log cleanup failed") {
		t.Fatalf("log does not report the failure: %s", buffer.String())
	}
	if !strings.Contains(buffer.String(), "notification-log cleanup completed") {
		t.Fatalf("log does not report the retry success: %s", buffer.String())
	}
}

func TestRunJobFailureLogsStableErrorAndContinues(t *testing.T) {
	cleaner := newFakeCleaner()
	cleaner.failFirst = true
	var buffer bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buffer, nil))

	cleanupJob(context.Background(), logger, notificationLogJob(cleaner.DeleteProcessedEventsBefore, 180*24*time.Hour), time.Now)

	output := buffer.String()
	if !strings.Contains(output, "notification-log cleanup failed") {
		t.Fatalf("log does not report the failure: %s", output)
	}
	if !strings.Contains(output, `operation=cleanup_notification_logs`) {
		t.Fatalf("log does not include the operation: %s", output)
	}
	if !strings.Contains(output, `error="secret database detail"`) {
		t.Fatalf("log does not include the stable error: %s", output)
	}
}

func TestRunJobDisabledWhenRetentionIsInvalid(t *testing.T) {
	var buffer bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buffer, nil))
	cleaner := newFakeCleaner()

	RunJob(context.Background(), logger, notificationLogJob(cleaner.DeleteProcessedEventsBefore, 0))

	if cleaner.callCount() != 0 {
		t.Fatalf("cleanup calls = %d, want 0", cleaner.callCount())
	}
	if !strings.Contains(buffer.String(), "notification-log cleanup disabled") {
		t.Fatalf("log does not report the disabled job: %s", buffer.String())
	}
}

func TestRunJobStopsWhenCanceledDuringCleanup(t *testing.T) {
	cleaner := newFakeCleaner()
	cleaner.release = make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	done := make(chan struct{})
	go func() {
		defer close(done)
		runJob(ctx, discardLogger(), notificationLogJob(cleaner.DeleteProcessedEventsBefore, 180*24*time.Hour), time.Now, make(chan time.Time))
	}()

	waitForCall(t, cleaner.calls)
	cancel()
	waitForDone(t, done)
	if cleaner.callCount() != 1 {
		t.Fatalf("cleanup calls = %d, want 1", cleaner.callCount())
	}
}

func TestRunJobDoesNotOverlapSlowCleanup(t *testing.T) {
	release := make(chan struct{})
	cleaner := newFakeCleaner()
	cleaner.release = release
	ticks := make(chan time.Time, 1)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	done := make(chan struct{})
	go func() {
		defer close(done)
		runJob(ctx, discardLogger(), notificationLogJob(cleaner.DeleteProcessedEventsBefore, 180*24*time.Hour), time.Now, ticks)
	}()

	waitForCall(t, cleaner.calls)
	ticks <- time.Now()
	if cleaner.callCount() != 1 {
		t.Fatalf("overlapping call detected: calls = %d, want 1", cleaner.callCount())
	}
	close(release)
	waitForCall(t, cleaner.calls)
	if cleaner.callCount() != 2 {
		t.Fatalf("cleanup calls = %d, want 2", cleaner.callCount())
	}
	cancel()
	waitForDone(t, done)
}

func TestRunJobLogsDurationAndDeletedCount(t *testing.T) {
	var buffer bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buffer, nil))
	cleaner := newFakeCleaner()

	cleanupJob(context.Background(), logger, notificationLogJob(cleaner.DeleteProcessedEventsBefore, 180*24*time.Hour), time.Now)

	output := buffer.String()
	if !strings.Contains(output, "notification-log cleanup completed") ||
		!strings.Contains(output, "deleted=1") ||
		!strings.Contains(output, "duration_ms=") {
		t.Fatalf("unexpected success log: %s", output)
	}
}

func TestRunJobCleanerErrorDoesNotLeakWhenLogErrorIsDisabled(t *testing.T) {
	var buffer bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buffer, nil))
	cleaner := newFakeCleaner()
	cleaner.failFirst = true

	// The processed-event policy keeps its existing log shape and never logs
	// the injected driver detail.
	cleanupJob(context.Background(), logger, processedEventJob(cleaner, 30*24*time.Hour, time.Hour), time.Now)

	output := buffer.String()
	if strings.Contains(output, "secret database detail") {
		t.Fatalf("processed-event log leaked the driver detail: %s", output)
	}
}

func TestRunJobCleanerErrorIsStable(t *testing.T) {
	err := errors.New("stable cleanup error")
	var buffer bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buffer, nil))

	cleanupJob(context.Background(), logger, notificationLogJob(func(context.Context, time.Time) (int64, error) {
		return 0, err
	}, 180*24*time.Hour), time.Now)

	if !strings.Contains(buffer.String(), err.Error()) {
		t.Fatalf("stable error missing from log: %s", buffer.String())
	}
}
