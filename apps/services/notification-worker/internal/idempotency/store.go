// Package idempotency records which notification events the worker has already
// processed so that redeliveries can be suppressed. It stores only the consumer
// name and event ID, never recipients or message contents.
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

// ErrUnavailable reports that the store has no database to query. It is a
// transient condition: callers must retry rather than acknowledge the message.
var ErrUnavailable = errors.New("processed-event store unavailable")

// DB is the subset of the shared PostgreSQL pool the store needs.
type DB interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// Postgres records processed event IDs in notification.processed_events.
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
