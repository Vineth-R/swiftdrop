// Package idempotency records notification delivery outcomes and the event IDs
// the worker has already processed. It stores only identifiers, delivery status
// and safe error text, never message bodies or other sensitive payload data.
package idempotency

import (
	"context"
	"errors"
	"fmt"
	"time"

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

// ErrUnavailable reports that the store has no database to query. It is a
// transient condition: callers must retry rather than acknowledge the message.
var ErrUnavailable = errors.New("processed-event store unavailable")

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

const logUpsertSQL = `
	INSERT INTO notification.notification_logs
		(event_id, parcel_id, channel, recipient, subject, status, error_message, sent_at)
	VALUES ($1, $2::uuid, $3, $4, $5, $6, $7, $8)
	ON CONFLICT (event_id, channel, recipient) DO UPDATE SET
		status        = EXCLUDED.status,
		subject       = EXCLUDED.subject,
		error_message = EXCLUDED.error_message,
		sent_at       = EXCLUDED.sent_at`

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
		return false, fmt.Errorf("query processed event: %w", err)
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

	_, err := p.db.Exec(ctx, `
		INSERT INTO notification.processed_events (consumer, event_id)
		VALUES ($1, $2::uuid)
		ON CONFLICT (consumer, event_id) DO NOTHING`, p.consumer, eventID)
	if err != nil {
		return fmt.Errorf("record processed event: %w", err)
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
		return fmt.Errorf("begin delivery transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, logUpsertSQL,
		entry.EventID, entry.ParcelID, entry.Channel, entry.Recipient, entry.Subject,
		StatusSent, nil, time.Now().UTC()); err != nil {
		return fmt.Errorf("upsert sent notification log: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO notification.processed_events (consumer, event_id)
		VALUES ($1, $2::uuid)
		ON CONFLICT (consumer, event_id) DO NOTHING`, p.consumer, entry.EventID); err != nil {
		return fmt.Errorf("record processed event: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit delivery transaction: %w", err)
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
		return fmt.Errorf("invalid failure status %q", entry.Status)
	}

	if _, err := p.db.Exec(ctx, logUpsertSQL,
		entry.EventID, entry.ParcelID, entry.Channel, entry.Recipient, entry.Subject,
		entry.Status, entry.ErrorMessage, nil); err != nil {
		return fmt.Errorf("upsert failed notification log: %w", err)
	}

	return nil
}
