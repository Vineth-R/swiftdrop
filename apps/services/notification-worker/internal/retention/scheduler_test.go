package retention

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"
)

type cleanupCall struct {
	cutoff time.Time
}

type fakeCleaner struct {
	calls     chan cleanupCall
	release   <-chan struct{}
	failFirst bool

	mu    sync.Mutex
	count int
}

func newFakeCleaner() *fakeCleaner {
	return &fakeCleaner{calls: make(chan cleanupCall, 64)}
}

func (f *fakeCleaner) DeleteProcessedEventsBefore(ctx context.Context, cutoff time.Time) (int64, error) {
	f.mu.Lock()
	f.count++
	call := f.count
	f.mu.Unlock()

	f.calls <- cleanupCall{cutoff: cutoff}
	if f.release != nil {
		select {
		case <-f.release:
		case <-ctx.Done():
			return 0, ctx.Err()
		}
	}
	if f.failFirst && call == 1 {
		return 0, errors.New("secret database detail")
	}
	return 1, nil
}

func (f *fakeCleaner) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.count
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func waitForCall(t *testing.T, calls <-chan cleanupCall) cleanupCall {
	t.Helper()
	select {
	case call := <-calls:
		return call
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for cleanup call")
		return cleanupCall{}
	}
}

func waitForDone(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for scheduler to stop")
	}
}

func TestRunCleansImmediately(t *testing.T) {
	fixedNow := time.Date(2026, time.November, 2, 12, 0, 0, 0, time.UTC)
	retention := 30 * 24 * time.Hour
	cleaner := newFakeCleaner()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	done := make(chan struct{})
	go func() {
		defer close(done)
		run(ctx, discardLogger(), cleaner, retention, func() time.Time { return fixedNow }, make(chan time.Time))
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

func TestRunCleansOnTicker(t *testing.T) {
	fixedNow := time.Date(2026, time.November, 2, 12, 0, 0, 0, time.UTC)
	cleaner := newFakeCleaner()
	ticks := make(chan time.Time, 1)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	done := make(chan struct{})
	go func() {
		defer close(done)
		run(ctx, discardLogger(), cleaner, 30*24*time.Hour, func() time.Time { return fixedNow }, ticks)
	}()

	waitForCall(t, cleaner.calls)
	ticks <- fixedNow
	waitForCall(t, cleaner.calls)
	if cleaner.callCount() != 2 {
		t.Fatalf("cleanup calls = %d, want 2", cleaner.callCount())
	}

	cancel()
	waitForDone(t, done)
}

func TestRunUsesConfiguredInterval(t *testing.T) {
	cleaner := newFakeCleaner()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	done := make(chan struct{})
	go func() {
		defer close(done)
		Run(ctx, discardLogger(), cleaner, 30*24*time.Hour, 20*time.Millisecond)
	}()

	waitForCall(t, cleaner.calls)
	waitForCall(t, cleaner.calls)
	if cleaner.callCount() < 2 {
		t.Fatalf("cleanup calls = %d, want at least 2", cleaner.callCount())
	}

	cancel()
	waitForDone(t, done)
}

func TestRunStopsWhenCanceled(t *testing.T) {
	cleaner := newFakeCleaner()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		run(ctx, discardLogger(), cleaner, 30*24*time.Hour, time.Now, make(chan time.Time))
	}()

	waitForDone(t, done)
	if cleaner.callCount() != 0 {
		t.Fatalf("cleanup calls = %d, want 0", cleaner.callCount())
	}
}

func TestRunContinuesAfterFailure(t *testing.T) {
	fixedNow := time.Date(2026, time.November, 2, 12, 0, 0, 0, time.UTC)
	cleaner := newFakeCleaner()
	cleaner.failFirst = true
	ticks := make(chan time.Time, 1)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	done := make(chan struct{})
	go func() {
		defer close(done)
		run(ctx, discardLogger(), cleaner, 30*24*time.Hour, func() time.Time { return fixedNow }, ticks)
	}()

	waitForCall(t, cleaner.calls)
	ticks <- fixedNow
	waitForCall(t, cleaner.calls)
	if cleaner.callCount() != 2 {
		t.Fatalf("cleanup calls = %d, want 2", cleaner.callCount())
	}

	cancel()
	waitForDone(t, done)
}

func TestRunDoesNotOverlapSlowCleanup(t *testing.T) {
	fixedNow := time.Date(2026, time.November, 2, 12, 0, 0, 0, time.UTC)
	release := make(chan struct{})
	cleaner := newFakeCleaner()
	cleaner.release = release
	ticks := make(chan time.Time, 1)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	done := make(chan struct{})
	go func() {
		defer close(done)
		run(ctx, discardLogger(), cleaner, 30*24*time.Hour, func() time.Time { return fixedNow }, ticks)
	}()

	waitForCall(t, cleaner.calls)
	ticks <- fixedNow
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

func TestRunStopsDuringSlowCleanup(t *testing.T) {
	cleaner := newFakeCleaner()
	cleaner.release = make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	done := make(chan struct{})
	go func() {
		defer close(done)
		run(ctx, discardLogger(), cleaner, 30*24*time.Hour, time.Now, make(chan time.Time))
	}()

	waitForCall(t, cleaner.calls)
	cancel()
	waitForDone(t, done)
	if cleaner.callCount() != 1 {
		t.Fatalf("cleanup calls = %d, want 1", cleaner.callCount())
	}
}

func TestCleanupLogsFailureWithoutDriverText(t *testing.T) {
	var buffer bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buffer, nil))
	cleaner := newFakeCleaner()
	cleaner.failFirst = true

	cleanup(context.Background(), logger, cleaner, 30*24*time.Hour, time.Now)

	output := buffer.String()
	if !strings.Contains(output, "processed-event cleanup failed") {
		t.Fatalf("log does not report the failure: %s", output)
	}
	if strings.Contains(output, "secret database detail") {
		t.Fatalf("log leaked the injected database detail: %s", output)
	}
}

func TestRunRejectsInvalidDependencies(t *testing.T) {
	var buffer bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buffer, nil))
	cleaner := newFakeCleaner()

	Run(context.Background(), logger, nil, 30*24*time.Hour, 24*time.Hour)
	Run(context.Background(), logger, cleaner, 0, 24*time.Hour)
	Run(context.Background(), logger, cleaner, 30*24*time.Hour, 0)

	if cleaner.callCount() != 0 {
		t.Fatalf("cleanup calls = %d, want 0", cleaner.callCount())
	}
	if !strings.Contains(buffer.String(), "processed-event cleanup disabled") {
		t.Fatalf("logs do not report disabled cleanup: %s", buffer.String())
	}
}
