package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"strings"
	"sync"
	"time"

	"github.com/Vineth-R/swiftdrop/packages/go-common/messaging"
	"github.com/Vineth-R/swiftdrop/packages/go-common/postgres"
)

// MaxAttempts is the maximum number of publish attempts before an event transitions to FAILED.
const MaxAttempts = 5

// claimStaleAfter is how long a claimed event may stay unprocessed before
// another relay replica may reclaim it. It must stay comfortably above the
// publisher's timeout (defaultPublishTimeout, 10s) so a live publisher is never
// preempted; a longer window only delays recovery from a crashed replica.
const claimStaleAfter = 2 * time.Minute

// ErrSkip indicates an event should not be published in this batch (e.g. currently in exponential backoff).
var ErrSkip = errors.New("outbox event skipped (in backoff)")

// BackoffDuration calculates exponential backoff with jitter for a given attempt count.
// Base delay is 1s, maximum delay is 30s.
func BackoffDuration(attempt int) time.Duration {
	if attempt <= 0 {
		return 0
	}
	shift := attempt - 1
	if shift > 5 {
		shift = 5
	}
	base := time.Second * (1 << shift)
	maxDelay := 30 * time.Second
	if base > maxDelay {
		base = maxDelay
	}

	half := base / 2
	if half <= 0 {
		return base
	}
	jitter := time.Duration(rand.Int64N(int64(half) + 1))
	return half + jitter
}

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

// claimedEvent is an Event plus the claim token issued by the claiming UPDATE.
// The token scopes every later state transition to the relay that owns the row.
type claimedEvent struct {
	Event
	claimToken string
}

// claimPendingSQL atomically claims a batch of PENDING events. The inner
// SELECT uses FOR UPDATE SKIP LOCKED so concurrent relay replicas take
// disjoint rows, and the stale cutoff lets a row whose claimant died
// mid-publish be reclaimed after claimStaleAfter. The claim is committed by
// this single statement, so no database row lock is held across the broker
// publish.
const claimPendingSQL = `
	UPDATE parcel.outbox_events AS e
	SET claimed_at = now(),
	    claim_token = gen_random_uuid()
	WHERE e.id IN (
		SELECT id
		FROM parcel.outbox_events
		WHERE status = 'PENDING'
		  AND (claimed_at IS NULL OR claimed_at < $1)
		ORDER BY created_at ASC
		LIMIT $2
		FOR UPDATE SKIP LOCKED
	)
	RETURNING e.id, e.event_type, e.routing_key, e.payload, e.attempts,
	          e.created_at, e.aggregate_type, e.aggregate_id, e.claim_token`

// ProcessPending claims a batch of PENDING events atomically, invokes handler
// for each claimed event without holding database row locks during the broker
// publish, and then records the outcome scoped to the claim token. Two relay
// replicas therefore never publish the same event: the claim UPDATE hands each
// row to exactly one of them.
func (s *SQLStore) ProcessPending(ctx context.Context, limit int, handler func(event Event) error) (int, error) {
	if s.pool == nil {
		return 0, errors.New("database pool is not configured")
	}
	if limit <= 0 {
		limit = 50
	}

	staleCutoff := time.Now().UTC().Add(-claimStaleAfter)
	rows, err := s.pool.Query(ctx, claimPendingSQL, staleCutoff, limit)
	if err != nil {
		return 0, fmt.Errorf("claim pending outbox events: %w", err)
	}

	var events []claimedEvent
	for rows.Next() {
		var c claimedEvent
		if err := rows.Scan(
			&c.ID,
			&c.EventType,
			&c.RoutingKey,
			&c.Payload,
			&c.Attempts,
			&c.CreatedAt,
			&c.AggregateType,
			&c.AggregateID,
			&c.claimToken,
		); err != nil {
			rows.Close()
			return 0, fmt.Errorf("scan claimed outbox event: %w", err)
		}
		c.Status = "PENDING"
		events = append(events, c)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, fmt.Errorf("iterate claimed outbox rows: %w", err)
	}
	rows.Close()

	processed := 0
	for _, event := range events {
		if event.Attempts >= MaxAttempts {
			const failMaxSQL = `
				UPDATE parcel.outbox_events
				SET status = 'FAILED',
				    claim_token = NULL,
				    claimed_at = NULL
				WHERE id = $1 AND claim_token = $2 AND status = 'PENDING'`
			tag, err := s.pool.Exec(ctx, failMaxSQL, event.ID, event.claimToken)
			if err != nil {
				return processed, fmt.Errorf("mark max-attempts event failed: %w", err)
			}
			if tag.RowsAffected() == 0 {
				// The claim was reclaimed by another relay; it owns the row now.
				continue
			}
			processed++
			continue
		}

		pubErr := handler(event.Event)
		if errors.Is(pubErr, ErrSkip) {
			// Release the claim without touching attempts; the relay's own
			// backoff map decides when this process tries again.
			const releaseSQL = `
				UPDATE parcel.outbox_events
				SET claim_token = NULL,
				    claimed_at = NULL
				WHERE id = $1 AND claim_token = $2 AND status = 'PENDING'`
			if _, err := s.pool.Exec(ctx, releaseSQL, event.ID, event.claimToken); err != nil {
				return processed, fmt.Errorf("release outbox claim: %w", err)
			}
			continue
		}

		if pubErr == nil {
			// Mark PUBLISHED only for the relay that still holds the claim.
			const publishedSQL = `
				UPDATE parcel.outbox_events
				SET status = 'PUBLISHED',
				    published_at = now(),
				    attempts = attempts + 1,
				    last_error = NULL,
				    claim_token = NULL,
				    claimed_at = NULL
				WHERE id = $1 AND claim_token = $2 AND status = 'PENDING'`
			tag, err := s.pool.Exec(ctx, publishedSQL, event.ID, event.claimToken)
			if err != nil {
				return processed, fmt.Errorf("mark event published: %w", err)
			}
			if tag.RowsAffected() == 0 {
				continue
			}
		} else {
			errMsg := pubErr.Error()
			newAttempts := event.Attempts + 1
			newStatus := "PENDING"
			if newAttempts >= MaxAttempts {
				newStatus = "FAILED"
			}
			const failedSQL = `
				UPDATE parcel.outbox_events
				SET status = $3,
				    attempts = $4,
				    last_error = $5,
				    claim_token = NULL,
				    claimed_at = NULL
				WHERE id = $1 AND claim_token = $2 AND status = 'PENDING'`
			tag, err := s.pool.Exec(ctx, failedSQL, event.ID, event.claimToken, newStatus, newAttempts, errMsg)
			if err != nil {
				return processed, fmt.Errorf("record event failure: %w", err)
			}
			if tag.RowsAffected() == 0 {
				continue
			}
		}
		processed++
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
	store        Store
	publisher    Publisher
	logger       *slog.Logger
	options      RelayOptions
	retryAfterMu sync.RWMutex
	retryAfter   map[string]time.Time
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
		store:      store,
		publisher:  publisher,
		logger:     logger,
		options:    opts,
		retryAfter: make(map[string]time.Time),
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
		r.retryAfterMu.RLock()
		until, inBackoff := r.retryAfter[event.ID]
		r.retryAfterMu.RUnlock()

		if inBackoff && time.Now().Before(until) {
			return ErrSkip
		}

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

			backoff := BackoffDuration(event.Attempts + 1)
			r.retryAfterMu.Lock()
			r.retryAfter[event.ID] = time.Now().Add(backoff)
			r.retryAfterMu.Unlock()
			return err
		}

		r.retryAfterMu.Lock()
		delete(r.retryAfter, event.ID)
		r.retryAfterMu.Unlock()

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
