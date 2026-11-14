package messaging

import (
	"sync"
	"sync/atomic"
	"time"
)

// ConsumerState is the lifecycle state of one running consumer.
//
// A consumer that is still dialling or has lost its broker session is not
// ready: it cannot process events until it reconnects. Readiness must only be
// reported after the broker connection, topology declaration and Consume call
// have all succeeded.
type ConsumerState string

const (
	// StateConnecting means the first broker session has not reached Consume.
	StateConnecting ConsumerState = "connecting"
	// StateReady means the consumer is attached to its queue and can process.
	StateReady ConsumerState = "ready"
	// StateReconnecting means an established session failed and is being rebuilt.
	StateReconnecting ConsumerState = "reconnecting"
	// StateStopped means the consumer has shut down or the context ended.
	StateStopped ConsumerState = "stopped"
)

// HealthSnapshot is a point-in-time view of a consumer's health.
type HealthSnapshot struct {
	State         ConsumerState
	Ready         bool
	ConnectedAt   time.Time
	LastError     string
	LastErrorAt   time.Time
	Transitions   uint64
	LastChangedAt time.Time
}

// Health tracks one consumer's readiness. It is safe for concurrent use and
// never exposes queue names or broker credentials: callers add their own
// context. A nil *Health is valid and silently does nothing, so consumers that
// do not report readiness keep working unchanged.
type Health struct {
	state         atomic.Value // ConsumerState
	connectedAt   atomic.Int64 // UnixNano, zero when never connected
	lastErrorAt   atomic.Int64 // UnixNano, zero when no error recorded
	transitions   atomic.Uint64
	lastChangedAt atomic.Int64 // UnixNano

	mu        sync.Mutex
	lastError string
}

// NewHealth returns a consumer health tracker in the connecting state.
func NewHealth() *Health {
	h := &Health{}
	h.MarkConnecting()
	return h
}

// MarkConnecting records that the consumer is (re)building its broker session.
func (h *Health) MarkConnecting() { h.store(StateConnecting, "") }

// MarkReady records that the consumer reached the Consume phase.
func (h *Health) MarkReady() { h.store(StateReady, "") }

// MarkReconnecting records a failed session and the reason, without the queue
// name or any broker credential.
func (h *Health) MarkReconnecting(err error) {
	message := ""
	if err != nil {
		message = err.Error()
	}
	h.store(StateReconnecting, message)
}

// MarkStopped records that the consumer has shut down.
func (h *Health) MarkStopped() { h.store(StateStopped, "") }

func (h *Health) store(state ConsumerState, lastError string) {
	if h == nil {
		return
	}
	h.state.Store(state)
	h.lastChangedAt.Store(time.Now().UTC().UnixNano())
	h.transitions.Add(1)
	if lastError != "" {
		h.mu.Lock()
		h.lastError = lastError
		h.mu.Unlock()
		h.lastErrorAt.Store(time.Now().UTC().UnixNano())
	}
	if state == StateReady {
		h.connectedAt.Store(time.Now().UTC().UnixNano())
	}
}

// State returns the current lifecycle state.
func (h *Health) State() ConsumerState {
	if h == nil {
		return StateReady
	}
	state, _ := h.state.Load().(ConsumerState)
	return state
}

// Ready reports whether the consumer is attached to the broker.
func (h *Health) Ready() bool {
	if h == nil {
		return true
	}
	state, _ := h.state.Load().(ConsumerState)
	return state == StateReady
}

// Snapshot returns a consistent copy of the current health.
func (h *Health) Snapshot() HealthSnapshot {
	if h == nil {
		return HealthSnapshot{State: StateReady, Ready: true}
	}
	state, _ := h.state.Load().(ConsumerState)
	h.mu.Lock()
	lastError := h.lastError
	h.mu.Unlock()
	return HealthSnapshot{
		State:         state,
		Ready:         state == StateReady,
		ConnectedAt:   unixNanoTime(h.connectedAt.Load()),
		LastError:     lastError,
		LastErrorAt:   unixNanoTime(h.lastErrorAt.Load()),
		Transitions:   h.transitions.Load(),
		LastChangedAt: unixNanoTime(h.lastChangedAt.Load()),
	}
}

func unixNanoTime(value int64) time.Time {
	if value == 0 {
		return time.Time{}
	}
	return time.Unix(0, value).UTC()
}
