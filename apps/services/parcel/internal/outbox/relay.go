package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/Vineth-R/swiftdrop/packages/go-common/messaging"
	"github.com/Vineth-R/swiftdrop/packages/go-common/postgres"
)

// Event is one row from parcel.outbox_events.
type Event struct {
	ID            string          `json:"id"`
	EventType     string          `json:"eventType"`
	RoutingKey    string          `json:"routingKey"`
	Payload       json.RawMessage `json:"payload"`
	Status        string          `json:"status"`
	Attempts      int             `json:"attempts"`
	LastError     *string         `json:"lastError,omitempty"`
	CreatedAt     time.Time       `json:"createdAt"`
	PublishedAt   *time.Time      `json:"publishedAt,omitempty"`
	AggregateType *string         `json:"aggregateType,omitempty"`
	AggregateID   *string         `json:"aggregateId,omitempty"`
}

// Publisher publishes an event payload to a message broker with publisher confirms.
type Publisher interface {
	Publish(ctx context.Context, exchange, routingKey string, body []byte) error
}

// Store defines persistence operations for the outbox relay.
type Store interface {
	ProcessPending(ctx context.Context, limit int, handler func(event Event) error) (int, error)
	CleanupPublished(ctx context.Context, olderThan time.Duration) (int64, error)
}

// SQLStore implements Store against a postgres.Pool.
type SQLStore struct {
	pool *postgres.Pool
}

// NewSQLStore constructs an SQLStore.
func NewSQLStore(pool *postgres.Pool) *SQLStore {
	return &SQLStore{pool: pool}
}

// ProcessPending locks a batch of PENDING events with FOR UPDATE SKIP LOCKED,
// invokes handler for each event, and updates the row status atomically.
func (s *SQLStore) ProcessPending(ctx context.Context, limit int, handler func(event Event) error) (int, error) {
	if s.pool == nil {
		return 0, errors.New("database pool is not configured")
	}
	if limit <= 0 {
		limit = 50
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	query := `
		SELECT id, event_type, routing_key, payload, attempts, created_at, aggregate_type, aggregate_id
		FROM parcel.outbox_events
		WHERE status = 'PENDING'
		ORDER BY created_at ASC
		LIMIT $1
		FOR UPDATE SKIP LOCKED`

	rows, err := tx.Query(ctx, query, limit)
	if err != nil {
		return 0, fmt.Errorf("fetch pending outbox events: %w", err)
	}
	defer rows.Close()

	var events []Event
	for rows.Next() {
		var e Event
		if err := rows.Scan(
			&e.ID,
			&e.EventType,
			&e.RoutingKey,
			&e.Payload,
			&e.Attempts,
			&e.CreatedAt,
			&e.AggregateType,
			&e.AggregateID,
		); err != nil {
			return 0, fmt.Errorf("scan outbox event: %w", err)
		}
		events = append(events, e)
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterate outbox rows: %w", err)
	}
	rows.Close()

	processed := 0
	for _, event := range events {
		pubErr := handler(event)
		if pubErr == nil {
			// Mark PUBLISHED
			updateSQL := `
				UPDATE parcel.outbox_events
				SET status = 'PUBLISHED',
				    published_at = now(),
				    attempts = attempts + 1,
				    last_error = NULL
				WHERE id = $1`
			if _, err := tx.Exec(ctx, updateSQL, event.ID); err != nil {
				return processed, fmt.Errorf("mark event published: %w", err)
			}
		} else {
			// Increment attempts and record last_error, leaving status PENDING for future retry
			errMsg := pubErr.Error()
			if len(errMsg) > 255 {
				errMsg = errMsg[:255]
			}
			updateSQL := `
				UPDATE parcel.outbox_events
				SET attempts = attempts + 1,
				    last_error = $2
				WHERE id = $1`
			if _, err := tx.Exec(ctx, updateSQL, event.ID, errMsg); err != nil {
				return processed, fmt.Errorf("record event failure: %w", err)
			}
		}
		processed++
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit outbox batch: %w", err)
	}
	return processed, nil
}

// CleanupPublished deletes PUBLISHED events older than the retention period.
func (s *SQLStore) CleanupPublished(ctx context.Context, retention time.Duration) (int64, error) {
	if s.pool == nil {
		return 0, errors.New("database pool is not configured")
	}
	if retention <= 0 {
		retention = 14 * 24 * time.Hour
	}

	cutoff := time.Now().UTC().Add(-retention)
	query := `DELETE FROM parcel.outbox_events WHERE status = 'PUBLISHED' AND published_at < $1`
	tag, err := s.pool.Exec(ctx, query, cutoff)
	if err != nil {
		return 0, fmt.Errorf("cleanup published outbox events: %w", err)
	}
	return tag.RowsAffected(), nil
}

// RelayOptions configures the outbox relay worker.
type RelayOptions struct {
	PollInterval    time.Duration
	CleanupInterval time.Duration
	RetentionPeriod time.Duration
	BatchSize       int
	Exchange        string
}

// DefaultRelayOptions provides production defaults.
func DefaultRelayOptions() RelayOptions {
	return RelayOptions{
		PollInterval:    1 * time.Second,
		CleanupInterval: 24 * time.Hour,
		RetentionPeriod: 14 * 24 * time.Hour,
		BatchSize:       50,
		Exchange:        "swiftdrop.events",
	}
}

// Relay polls pending outbox events and publishes them to the message broker.
type Relay struct {
	store     Store
	publisher Publisher
	logger    *slog.Logger
	options   RelayOptions
}

// NewRelay returns an initialized Relay worker.
func NewRelay(store Store, publisher Publisher, logger *slog.Logger, options ...RelayOptions) *Relay {
	opts := DefaultRelayOptions()
	if len(options) > 0 {
		opts = options[0]
	}
	if opts.PollInterval <= 0 {
		opts.PollInterval = 1 * time.Second
	}
	if opts.BatchSize <= 0 {
		opts.BatchSize = 50
	}
	if opts.Exchange == "" {
		opts.Exchange = "swiftdrop.events"
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Relay{
		store:     store,
		publisher: publisher,
		logger:    logger,
		options:   opts,
	}
}

// Run executes the relay background loop until ctx is cancelled.
func (r *Relay) Run(ctx context.Context) {
	r.logger.Info("starting outbox relay worker",
		"exchange", r.options.Exchange,
		"pollInterval", r.options.PollInterval,
		"retention", r.options.RetentionPeriod,
	)

	// Run initial cleanup once at startup
	if r.store != nil {
		deleted, err := r.store.CleanupPublished(ctx, r.options.RetentionPeriod)
		if err != nil {
			r.logger.Warn("outbox cleanup failed at startup", "error", err)
		} else if deleted > 0 {
			r.logger.Info("outbox cleanup completed", "deletedRows", deleted)
		}
	}

	pollTicker := time.NewTicker(r.options.PollInterval)
	defer pollTicker.Stop()

	cleanupInterval := r.options.CleanupInterval
	if cleanupInterval <= 0 {
		cleanupInterval = 24 * time.Hour
	}
	cleanupTicker := time.NewTicker(cleanupInterval)
	defer cleanupTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			r.logger.Info("stopping outbox relay worker")
			return

		case <-cleanupTicker.C:
			if r.store != nil {
				deleted, err := r.store.CleanupPublished(ctx, r.options.RetentionPeriod)
				if err != nil {
					r.logger.Warn("periodic outbox cleanup failed", "error", err)
				} else if deleted > 0 {
					r.logger.Info("periodic outbox cleanup completed", "deletedRows", deleted)
				}
			}

		case <-pollTicker.C:
			r.pollOnce(ctx)
		}
	}
}

// PollOnce processes one batch of pending events. Exported for testing and synchronous execution.
func (r *Relay) PollOnce(ctx context.Context) (int, error) {
	return r.pollOnce(ctx)
}

func (r *Relay) pollOnce(ctx context.Context) (int, error) {
	if r.store == nil || r.publisher == nil {
		return 0, nil
	}

	count, err := r.store.ProcessPending(ctx, r.options.BatchSize, func(event Event) error {
		publishedType, publishedKey := mapEventRouting(event.EventType, event.RoutingKey)

		// Verify data is a valid JSON object
		data := event.Payload
		if len(data) == 0 || data[0] != '{' {
			return errors.New("event payload must be a JSON object")
		}

		envelope := messaging.Envelope{
			ID:         event.ID,
			Type:       publishedType,
			Version:    1,
			OccurredAt: event.CreatedAt.UTC(),
			Producer:   "parcel",
			Data:       data,
		}

		if err := envelope.Validate(); err != nil {
			return fmt.Errorf("invalid canonical envelope: %w", err)
		}

		body, err := json.Marshal(envelope)
		if err != nil {
			return fmt.Errorf("marshal envelope: %w", err)
		}

		if err := r.publisher.Publish(ctx, r.options.Exchange, publishedKey, body); err != nil {
			r.logger.Warn("failed to publish outbox event",
				"eventId", event.ID,
				"eventType", publishedType,
				"routingKey", publishedKey,
				"error", err,
			)
			return err
		}

		r.logger.Info("published outbox event",
			"eventId", event.ID,
			"eventType", publishedType,
			"routingKey", publishedKey,
		)
		return nil
	})

	if err != nil && !errors.Is(err, context.Canceled) {
		r.logger.Warn("error processing outbox batch", "error", err)
	}
	return count, err
}

// mapEventRouting converts stored internal labels to canonical published event types and routing keys (API v1.1 Section 11.4).
func mapEventRouting(eventType, route string) (publishedType string, publishedRoutingKey string) {
	switch {
	case eventType == "parcel.created" || eventType == "parcel.created.v1" || route == "parcel.created" || route == "parcel.created.v1":
		return "parcel.created.v1", "parcel.created.v1"

	case eventType == "parcel.assigned" || eventType == "parcel.assigned.v1" || route == "parcel.assigned" || route == "parcel.assigned.v1":
		return "parcel.assigned.v1", "parcel.assigned.v1"

	case eventType == "parcel.status_changed" || eventType == "parcel.status_changed.v1" || route == "parcel.status.changed" || route == "parcel.status_changed.v1":
		return "parcel.status_changed.v1", "parcel.status_changed.v1"

	default:
		// Suffix with .v1 if not present
		cleanType := strings.TrimSpace(eventType)
		if !strings.HasSuffix(cleanType, ".v1") {
			cleanType += ".v1"
		}
		cleanRoute := strings.TrimSpace(route)
		if !strings.HasSuffix(cleanRoute, ".v1") {
			cleanRoute += ".v1"
		}
		return cleanType, cleanRoute
	}
}
