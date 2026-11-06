//go:build integration

package idempotency

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Vineth-R/swiftdrop/packages/go-common/postgres"
)

func TestPostgresDeduplicatesConcurrentInserts(t *testing.T) {
	ctx, db := openTestDatabase(t)
	consumer := "notification-worker-test-" + randomHex(6)
	eventID := randomUUID(t)
	t.Cleanup(func() { cleanupOutcomes(t, db, consumer, eventID) })

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

func TestPostgresRecordsDeliveryOutcome(t *testing.T) {
	ctx, db := openTestDatabase(t)
	consumer := "notification-worker-test-" + randomHex(6)
	eventID := randomUUID(t)
	recipient := "m1." + randomHex(6) + "@example.invalid"
	t.Cleanup(func() { cleanupOutcomes(t, db, consumer, eventID) })

	store := NewPostgres(db, consumer)
	entry := LogEntry{
		EventID:   eventID,
		ParcelID:  randomUUID(t),
		Channel:   "EMAIL",
		Recipient: recipient,
		Subject:   "Your parcel has shipped",
	}

	entry.Status = StatusPending
	entry.ErrorMessage = "smtp connection refused"
	if err := store.RecordFailure(ctx, entry); err != nil {
		t.Fatalf("RecordFailure() returned error: %v", err)
	}

	status, errorMessage, sentAt := readLog(t, ctx, db, eventID, entry.Channel, recipient)
	if status != StatusPending {
		t.Fatalf("status after failure = %q, want PENDING", status)
	}
	if errorMessage != "smtp connection refused" {
		t.Fatalf("error message = %q", errorMessage)
	}
	if sentAt != nil {
		t.Fatalf("sent_at = %v, want nil", sentAt)
	}
	if processed, err := store.IsProcessed(ctx, eventID); err != nil || processed {
		t.Fatalf("failure marked the event processed: processed=%v err=%v", processed, err)
	}

	if err := store.RecordDelivered(ctx, entry); err != nil {
		t.Fatalf("RecordDelivered() returned error: %v", err)
	}

	status, errorMessage, sentAt = readLog(t, ctx, db, eventID, entry.Channel, recipient)
	if status != StatusSent {
		t.Fatalf("status after delivery = %q, want SENT", status)
	}
	if errorMessage != "" {
		t.Fatalf("error message after delivery = %q, want empty", errorMessage)
	}
	if sentAt == nil {
		t.Fatal("sent_at was not set on delivery")
	}
	if processed, err := store.IsProcessed(ctx, eventID); err != nil || !processed {
		t.Fatalf("delivery did not mark the event processed: processed=%v err=%v", processed, err)
	}

	var count int
	if err := db.QueryRow(ctx,
		`SELECT count(*) FROM notification.notification_logs WHERE event_id = $1::uuid`, eventID).
		Scan(&count); err != nil {
		t.Fatalf("count notification logs: %v", err)
	}
	if count != 1 {
		t.Fatalf("notification log rows = %d, want 1", count)
	}
}

func TestPostgresRecordDeliveredConcurrent(t *testing.T) {
	ctx, db := openTestDatabase(t)
	consumer := "notification-worker-test-" + randomHex(6)
	eventID := randomUUID(t)
	recipient := "m1." + randomHex(6) + "@example.invalid"
	t.Cleanup(func() { cleanupOutcomes(t, db, consumer, eventID) })

	store := NewPostgres(db, consumer)
	entry := LogEntry{
		EventID:   eventID,
		ParcelID:  randomUUID(t),
		Channel:   "EMAIL",
		Recipient: recipient,
		Subject:   "Your parcel has shipped",
	}

	const workers = 8
	var wait sync.WaitGroup
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			errs <- store.RecordDelivered(ctx, entry)
		}()
	}
	wait.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent RecordDelivered() failed: %v", err)
		}
	}

	var logCount int
	if err := db.QueryRow(ctx, `
		SELECT count(*) FROM notification.notification_logs
		WHERE event_id = $1::uuid AND channel = $2 AND recipient = $3`,
		eventID, entry.Channel, recipient).Scan(&logCount); err != nil {
		t.Fatalf("count notification logs: %v", err)
	}
	if logCount != 1 {
		t.Fatalf("notification log rows = %d, want 1", logCount)
	}

	var processedCount int
	if err := db.QueryRow(ctx, `
		SELECT count(*) FROM notification.processed_events
		WHERE consumer = $1 AND event_id = $2::uuid`, consumer, eventID).Scan(&processedCount); err != nil {
		t.Fatalf("count processed events: %v", err)
	}
	if processedCount != 1 {
		t.Fatalf("processed rows = %d, want 1", processedCount)
	}

	status, _, sentAt := readLog(t, ctx, db, eventID, entry.Channel, recipient)
	if status != StatusSent {
		t.Fatalf("status = %q, want SENT", status)
	}
	if sentAt == nil {
		t.Fatal("sent_at was not set")
	}
}

func TestPostgresRecordFailurePreservesSentAt(t *testing.T) {
	ctx, db := openTestDatabase(t)
	consumer := "notification-worker-test-" + randomHex(6)
	eventID := randomUUID(t)
	recipient := "m1." + randomHex(6) + "@example.invalid"
	t.Cleanup(func() { cleanupOutcomes(t, db, consumer, eventID) })

	store := NewPostgres(db, consumer)
	entry := LogEntry{
		EventID:   eventID,
		ParcelID:  randomUUID(t),
		Channel:   "EMAIL",
		Recipient: recipient,
		Subject:   "Your parcel has shipped",
	}

	entry.Status = StatusFailed
	entry.ErrorMessage = "first failure"
	if err := store.RecordFailure(ctx, entry); err != nil {
		t.Fatalf("first RecordFailure() returned error: %v", err)
	}
	status, _, sentAt := readLog(t, ctx, db, eventID, entry.Channel, recipient)
	if status != StatusFailed {
		t.Fatalf("status = %q, want FAILED", status)
	}
	if sentAt != nil {
		t.Fatalf("first failure sent_at = %v, want nil", sentAt)
	}

	if err := store.RecordDelivered(ctx, entry); err != nil {
		t.Fatalf("RecordDelivered() returned error: %v", err)
	}
	status, _, sentAt = readLog(t, ctx, db, eventID, entry.Channel, recipient)
	if status != StatusSent {
		t.Fatalf("status = %q, want SENT", status)
	}
	if sentAt == nil {
		t.Fatal("sent_at was not set on delivery")
	}
	deliveredAt := *sentAt

	entry.Status = StatusPending
	entry.ErrorMessage = "later failure"
	if err := store.RecordFailure(ctx, entry); err != nil {
		t.Fatalf("later RecordFailure() returned error: %v", err)
	}
	status, errorMessage, sentAt := readLog(t, ctx, db, eventID, entry.Channel, recipient)
	if status != StatusPending {
		t.Fatalf("status after later failure = %q, want PENDING", status)
	}
	if errorMessage != "later failure" {
		t.Fatalf("error message = %q", errorMessage)
	}
	if sentAt == nil {
		t.Fatal("later failure erased sent_at")
	}
	if !sentAt.Equal(deliveredAt) {
		t.Fatalf("sent_at = %v, want preserved %v", sentAt, deliveredAt)
	}
}

func TestPostgresRecordDeliveredRollsBackWhenProcessedInsertFails(t *testing.T) {
	ctx, db := openTestDatabase(t)
	consumer := strings.Repeat("c", 41)
	eventID := randomUUID(t)
	recipient := "m1." + randomHex(6) + "@example.invalid"
	t.Cleanup(func() { cleanupOutcomes(t, db, consumer, eventID) })

	store := NewPostgres(db, consumer)
	entry := LogEntry{
		EventID:   eventID,
		ParcelID:  randomUUID(t),
		Channel:   "EMAIL",
		Recipient: recipient,
		Subject:   "Your parcel has shipped",
	}

	if err := store.RecordDelivered(ctx, entry); err == nil {
		t.Fatal("RecordDelivered() should fail when the processed consumer exceeds its column")
	}

	var count int
	if err := db.QueryRow(ctx,
		`SELECT count(*) FROM notification.notification_logs WHERE event_id = $1::uuid`, eventID).
		Scan(&count); err != nil {
		t.Fatalf("count notification logs: %v", err)
	}
	if count != 0 {
		t.Fatalf("notification log rows = %d, want 0 after rollback", count)
	}
}

func TestPostgresEnforcesLogChannelAndStatusValues(t *testing.T) {
	ctx, db := openTestDatabase(t)
	consumer := "notification-worker-test-" + randomHex(6)
	eventID := randomUUID(t)
	recipient := "m1." + randomHex(6) + "@example.invalid"
	t.Cleanup(func() { cleanupOutcomes(t, db, consumer, eventID) })

	store := NewPostgres(db, consumer)
	entry := LogEntry{
		EventID:      eventID,
		ParcelID:     randomUUID(t),
		Channel:      "email",
		Recipient:    recipient,
		Subject:      "Your parcel has shipped",
		Status:       StatusFailed,
		ErrorMessage: "unsupported notification channel",
	}

	if err := store.RecordFailure(ctx, entry); err == nil {
		t.Fatal("RecordFailure() accepted a lowercase channel")
	}

	entry.Channel = "SMS"
	if err := store.RecordFailure(ctx, entry); err != nil {
		t.Fatalf("RecordFailure() rejected SMS: %v", err)
	}

	status, _, _ := readLog(t, ctx, db, eventID, "SMS", recipient)
	if status != StatusFailed {
		t.Fatalf("status = %q, want FAILED", status)
	}
}

func openTestDatabase(t *testing.T) (context.Context, *postgres.Pool) {
	t.Helper()

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
		CREATE TABLE IF NOT EXISTS notification.notification_logs (
			id            UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
			event_id      UUID         NOT NULL,
			parcel_id     UUID         NOT NULL,
			channel       VARCHAR(10)  NOT NULL CHECK (channel IN ('EMAIL','SMS')),
			recipient     VARCHAR(255) NOT NULL,
			subject       VARCHAR(200),
			status        VARCHAR(10)  NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING','SENT','FAILED')),
			error_message VARCHAR(255),
			created_at    TIMESTAMPTZ  NOT NULL DEFAULT now(),
			sent_at       TIMESTAMPTZ,
			CONSTRAINT notification_dedupe_uq UNIQUE (event_id, channel, recipient)
		)`); err != nil {
		t.Fatalf("create notification_logs table: %v", err)
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

	return ctx, db
}

func readLog(t *testing.T, ctx context.Context, db *postgres.Pool, eventID, channel, recipient string) (string, string, *time.Time) {
	t.Helper()

	var status string
	var errorMessage *string
	var sentAt *time.Time
	if err := db.QueryRow(ctx, `
		SELECT status, error_message, sent_at
		FROM notification.notification_logs
		WHERE event_id = $1::uuid AND channel = $2 AND recipient = $3`,
		eventID, channel, recipient).Scan(&status, &errorMessage, &sentAt); err != nil {
		t.Fatalf("read notification log: %v", err)
	}

	message := ""
	if errorMessage != nil {
		message = *errorMessage
	}
	return status, message, sentAt
}

func cleanupOutcomes(t *testing.T, db *postgres.Pool, consumer, eventID string) {
	t.Helper()

	cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := db.Exec(cleanupCtx,
		`DELETE FROM notification.notification_logs WHERE event_id = $1::uuid`, eventID); err != nil {
		t.Logf("delete notification logs: %v", err)
	}
	if _, err := db.Exec(cleanupCtx,
		`DELETE FROM notification.processed_events WHERE consumer = $1`, consumer); err != nil {
		t.Logf("delete processed events: %v", err)
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
