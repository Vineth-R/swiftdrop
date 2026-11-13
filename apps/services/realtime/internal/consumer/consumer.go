// Package consumer turns parcel.status_changed.v1 broker events into live
// updates for connected WebSocket clients.
package consumer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/Vineth-R/swiftdrop/apps/services/realtime/internal/live"
	"github.com/Vineth-R/swiftdrop/packages/go-common/messaging"
)

// EventType is the only event this consumer understands.
const EventType = "parcel.status_changed.v1"

// Broadcaster delivers an update to the clients following it. *live.Hub
// satisfies it.
type Broadcaster interface {
	Broadcast(update live.Update) int
}

const (
	defaultDedupeCapacity = 4096
	defaultDedupeTTL      = 10 * time.Minute
)

// Handler implements messaging.Handler.
type Handler struct {
	hub    Broadcaster
	dedupe *eventDedupe
}

// New returns a consumer with the default bounded deduplication window.
func New(hub Broadcaster) *Handler {
	return NewWithDedupe(hub, defaultDedupeCapacity, defaultDedupeTTL)
}

// NewWithDedupe returns a consumer that suppresses a repeated event ID within
// a bounded, process-local window. Deduplication is not shared across replicas
// and does not survive a restart; durable replay remains future work.
func NewWithDedupe(hub Broadcaster, capacity int, ttl time.Duration) *Handler {
	if capacity < 1 {
		capacity = defaultDedupeCapacity
	}
	if ttl <= 0 {
		ttl = defaultDedupeTTL
	}
	return &Handler{hub: hub, dedupe: &eventDedupe{capacity: capacity, ttl: ttl, seen: make(map[string]time.Time)}}
}

type eventDedupe struct {
	mu       sync.Mutex
	capacity int
	ttl      time.Duration
	seen     map[string]time.Time
	order    []string
}

// claim records the event ID and reports whether it is new. The caller sends
// only when claim returns true.
func (d *eventDedupe) claim(id string, now time.Time) bool {
	if id == "" {
		return true
	}
	d.mu.Lock()
	defer d.mu.Unlock()

	for len(d.order) > 0 {
		oldest := d.order[0]
		expiry, ok := d.seen[oldest]
		if ok && now.Before(expiry) {
			break
		}
		d.order = d.order[1:]
		delete(d.seen, oldest)
	}
	if _, exists := d.seen[id]; exists {
		return false
	}
	d.seen[id] = now.Add(d.ttl)
	d.order = append(d.order, id)
	for len(d.order) > d.capacity {
		oldest := d.order[0]
		d.order = d.order[1:]
		delete(d.seen, oldest)
	}
	return true
}

// payload lists only the fields API v1.1 section 9 lets clients see. Anything
// else in the event (merchant_id, customer_user_id, parcel_id, note,
// failure_reason, ...) is never decoded, so it cannot be forwarded.
type payload struct {
	TrackingNumber string `json:"tracking_number"`
	PreviousStatus string `json:"previous_status"`
	Status         string `json:"status"`
}

// Handle decodes one message. Malformed or unexpected events are permanent
// errors: retrying cannot fix them.
func (h *Handler) Handle(_ context.Context, message messaging.Message) error {
	var envelope messaging.Envelope
	if err := json.NewDecoder(bytes.NewReader(message.Body)).Decode(&envelope); err != nil {
		return messaging.Permanent(fmt.Errorf("decode envelope: %w", err))
	}
	if err := envelope.Validate(); err != nil {
		return messaging.Permanent(fmt.Errorf("invalid envelope: %w", err))
	}
	if envelope.Type != EventType {
		return messaging.Permanent(fmt.Errorf("unexpected event type %q", envelope.Type))
	}

	var data payload
	if err := json.Unmarshal(envelope.Data, &data); err != nil {
		return messaging.Permanent(fmt.Errorf("decode data: %w", err))
	}
	if !live.ValidTrackingNumber(data.TrackingNumber) {
		return messaging.Permanent(fmt.Errorf("invalid tracking_number %q", data.TrackingNumber))
	}
	if !live.ValidStatus(data.Status) {
		return messaging.Permanent(fmt.Errorf("invalid status %q", data.Status))
	}
	if !live.ValidStatus(data.PreviousStatus) {
		return messaging.Permanent(fmt.Errorf("invalid previous_status %q", data.PreviousStatus))
	}

	if !h.dedupe.claim(envelope.ID, time.Now()) {
		return nil
	}
	h.hub.Broadcast(live.Update{
		TrackingNumber: data.TrackingNumber,
		PreviousStatus: data.PreviousStatus,
		Status:         data.Status,
		UpdatedAt:      envelope.OccurredAt,
	})
	return nil
}
