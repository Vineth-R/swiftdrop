// Package idempotency records notification delivery outcomes and the event IDs
// the worker has already processed. The notification_logs schema requires the
// recipient and subject, so those values are persisted; the worker sanitizes
// the error text it stores. Database-driver errors are never logged or returned
// from this package: it reports only stable operation errors.
package idempotency

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// ConsumerName is the value this worker writes to processed_events.consumer.
const ConsumerName = "notification-worker"

// Delivery statuses accepted by notification.notification_logs.
const (
	StatusPending = "PENDING"
	StatusSent    = "SENT"
	StatusFailed  = "FAILED"
)

// Stable, non-sensitive errors. Driver messages are deliberately discarded so
// a PostgreSQL failing-row detail can never reach logs or consumer error text.
var (
	ErrUnavailable           = errors.New("processed-event store unavailable")
	ErrLookupFailed          = errors.New("processed-event lookup failed")
	ErrMarkProcessedFailed   = errors.New("record processed event failed")
	ErrRecordDeliveredFailed = errors.New("record delivered notification failed")
	ErrRecordFailureFailed   = errors.New("record failed notification failed")
	ErrCleanupFailed         = errors.New("processed-event cleanup failed")
)

// DB is the subset of the shared PostgreSQL pool the store needs.
type DB interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Begin(ctx context.Context) (pgx.Tx, error)
}

// LogEntry is one notification delivery outcome.
type LogEntry struct {
	EventID      string
	ParcelID     string
	Channel      string
	Recipient    string
	Subject      string
	Status       string
	ErrorMessage string
}

const processedInsertSQL = `
	INSERT INTO notification.processed_events (consumer, event_id)
	VALUES ($1, $2::uuid)
	ON CONFLICT (consumer, event_id) DO NOTHING`

// logDeliveredSQL uses the database clock for sent_at and preserves created_at
// on conflict.
const logDeliveredSQL = `
	INSERT INTO notification.notification_logs
		(event_id, parcel_id, channel, recipient, subject, status, error_message, sent_at)
	VALUES ($1, $2::uuid, $3, $4, $5, 'SENT', NULL, now())
	ON CONFLICT (event_id, channel, recipient) DO UPDATE SET
		status        = 'SENT',
		subject       = EXCLUDED.subject,
		error_message = NULL,
		sent_at       = now()`

// logFailureSQL preserves an existing successful timestamp when a later failure
// is recorded for the same (event_id, channel, recipient).
const logFailureSQL = `
	INSERT INTO notification.notification_logs
		(event_id, parcel_id, channel, recipient, subject, status, error_message, sent_at)
	VALUES ($1, $2::uuid, $3, $4, $5, $6, $7, NULL)
	ON CONFLICT (event_id, channel, recipient) DO UPDATE SET
		status        = EXCLUDED.status,
		subject       = EXCLUDED.subject,
		error_message = EXCLUDED.error_message,
		sent_at       = COALESCE(notification_logs.sent_at, EXCLUDED.sent_at)`

// Postgres records processed event IDs and delivery outcomes in the
// notification schema.
type Postgres struct {
	db       DB
	consumer string
}

// NewPostgres returns a store that uses consumer to scope processed event IDs.
func NewPostgres(db DB, consumer string) *Postgres {
	return &Postgres{db: db, consumer: consumer}
}

// IsProcessed reports whether eventID has already been recorded by this
// consumer.
func (p *Postgres) IsProcessed(ctx context.Context, eventID string) (bool, error) {
	if p == nil || p.db == nil {
		return false, ErrUnavailable
	}

	var processed bool
	err := p.db.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM notification.processed_events
			WHERE consumer = $1 AND event_id = $2::uuid
		)`, p.consumer, eventID).Scan(&processed)
	if err != nil {
		return false, fmt.Errorf("%w: query", ErrLookupFailed)
	}

	return processed, nil
}

// MarkProcessed records eventID as successfully processed. A concurrent insert
// for the same (consumer, event_id) is ignored, so the primary key prevents
// duplicate records.
func (p *Postgres) MarkProcessed(ctx context.Context, eventID string) error {
	if p == nil || p.db == nil {
		return ErrUnavailable
	}

	if _, err := p.db.Exec(ctx, processedInsertSQL, p.consumer, eventID); err != nil {
		return fmt.Errorf("%w: insert", ErrMarkProcessedFailed)
	}

	return nil
}

// RecordDelivered atomically records a SENT notification log and the processed
// event. It commits only after both writes succeed.
func (p *Postgres) RecordDelivered(ctx context.Context, entry LogEntry) error {
	if p == nil || p.db == nil {
		return ErrUnavailable
	}

	tx, err := p.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("%w: begin transaction", ErrRecordDeliveredFailed)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, logDeliveredSQL,
		entry.EventID, entry.ParcelID, entry.Channel, entry.Recipient, entry.Subject); err != nil {
		return fmt.Errorf("%w: upsert log", ErrRecordDeliveredFailed)
	}

	if _, err := tx.Exec(ctx, processedInsertSQL, p.consumer, entry.EventID); err != nil {
		return fmt.Errorf("%w: insert processed event", ErrRecordDeliveredFailed)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("%w: commit", ErrRecordDeliveredFailed)
	}

	return nil
}

// RecordFailure records a PENDING (retryable) or FAILED (permanent) delivery
// outcome. It never records the event as processed.
func (p *Postgres) RecordFailure(ctx context.Context, entry LogEntry) error {
	if p == nil || p.db == nil {
		return ErrUnavailable
	}
	if entry.Status != StatusPending && entry.Status != StatusFailed {
		return fmt.Errorf("%w: invalid status", ErrRecordFailureFailed)
	}

	if _, err := p.db.Exec(ctx, logFailureSQL,
		entry.EventID, entry.ParcelID, entry.Channel, entry.Recipient, entry.Subject,
		entry.Status, entry.ErrorMessage); err != nil {
		return fmt.Errorf("%w: upsert log", ErrRecordFailureFailed)
	}

	return nil
}
