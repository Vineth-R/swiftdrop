//go:build integration

package idempotency

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/Vineth-R/swiftdrop/packages/go-common/postgres"
)

func TestPostgresDeduplicatesConcurrentInserts(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)

	config := postgres.DefaultConfig()
	config.URL = url
	db, err := postgres.Open(ctx, config)
	if err != nil {
		t.Fatalf("connect to test database: %v", err)
	}
	t.Cleanup(db.Close)

	if _, err := db.Exec(ctx, `CREATE SCHEMA IF NOT EXISTS notification`); err != nil {
		t.Fatalf("create notification schema: %v", err)
	}
	if _, err := db.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS notification.processed_events (
			consumer     VARCHAR(40) NOT NULL,
			event_id     UUID        NOT NULL,
			processed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			PRIMARY KEY (consumer, event_id)
		)`); err != nil {
		t.Fatalf("create processed_events table: %v", err)
	}

	consumer := "notification-worker-test-" + randomHex(6)
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		if _, err := db.Exec(cleanupCtx,
			`DELETE FROM notification.processed_events WHERE consumer = $1`, consumer); err != nil {
			t.Logf("delete test rows: %v", err)
		}
	})

	eventID := randomUUID(t)
	store := NewPostgres(db, consumer)

	processed, err := store.IsProcessed(ctx, eventID)
	if err != nil {
		t.Fatalf("IsProcessed() before insert: %v", err)
	}
	if processed {
		t.Fatal("event was already processed before the test inserted it")
	}

	const workers = 20
	var wait sync.WaitGroup
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			errs <- store.MarkProcessed(ctx, eventID)
		}()
	}
	wait.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent MarkProcessed() failed: %v", err)
		}
	}

	var count int
	if err := db.QueryRow(ctx,
		`SELECT count(*) FROM notification.processed_events WHERE consumer = $1`, consumer).
		Scan(&count); err != nil {
		t.Fatalf("count processed rows: %v", err)
	}
	if count != 1 {
		t.Fatalf("processed rows = %d, want 1", count)
	}

	processed, err = store.IsProcessed(ctx, eventID)
	if err != nil {
		t.Fatalf("IsProcessed() after insert: %v", err)
	}
	if !processed {
		t.Fatal("event was not recorded as processed")
	}
}

func randomHex(size int) string {
	buffer := make([]byte, size)
	if _, err := rand.Read(buffer); err != nil {
		panic(err)
	}
	return hex.EncodeToString(buffer)
}

func randomUUID(t *testing.T) string {
	t.Helper()

	var buffer [16]byte
	if _, err := rand.Read(buffer[:]); err != nil {
		t.Fatalf("generate random UUID: %v", err)
	}
	return fmt.Sprintf("%x-%x-%x-%x-%x",
		buffer[0:4], buffer[4:6], buffer[6:8], buffer[8:10], buffer[10:16])
}
