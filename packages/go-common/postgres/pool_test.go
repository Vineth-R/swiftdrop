package postgres

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestOpenWithRetrySucceedsAfterFailures(t *testing.T) {
	attempts := 0
	connect := func(_ context.Context, _ Config) (*Pool, error) {
		attempts++
		if attempts < 3 {
			return nil, errors.New("database not ready")
		}
		return &Pool{}, nil
	}

	config := DefaultConfig()
	config.URL = "postgres://user:secret@db:5432/swiftdrop"
	config.RetryAttempts = 5
	config.RetryMin = time.Millisecond
	config.RetryMax = time.Millisecond

	if _, err := openWithRetry(context.Background(), config, connect); err != nil {
		t.Fatalf("openWithRetry() error: %v", err)
	}
	if attempts != 3 {
		t.Fatalf("attempts = %d, want 3", attempts)
	}
}

func TestOpenWithRetryExhaustsAttempts(t *testing.T) {
	attempts := 0
	connect := func(_ context.Context, _ Config) (*Pool, error) {
		attempts++
		return nil, errors.New("database not ready")
	}

	config := DefaultConfig()
	config.URL = "postgres://user:secret@db:5432/swiftdrop"
	config.RetryAttempts = 3
	config.RetryMin = time.Millisecond
	config.RetryMax = time.Millisecond

	_, err := openWithRetry(context.Background(), config, connect)
	if err == nil {
		t.Fatal("expected error after exhausting attempts")
	}
	if attempts != 3 {
		t.Fatalf("attempts = %d, want 3", attempts)
	}
}

func TestOpenWithRetryStopsOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	config := DefaultConfig()
	config.URL = "postgres://user:secret@db:5432/swiftdrop"
	config.RetryAttempts = 5
	config.RetryMin = time.Hour
	config.RetryMax = time.Hour

	connect := func(_ context.Context, _ Config) (*Pool, error) {
		return nil, errors.New("database not ready")
	}

	_, err := openWithRetry(ctx, config, connect)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestOpenRejectsUnconfigured(t *testing.T) {
	if _, err := Open(context.Background(), Config{}); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("expected ErrNotConfigured, got %v", err)
	}
}
