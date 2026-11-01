package mailer

import (
	"context"
	"sync"
)

// Mock is a Mailer for tests: it records every message instead of sending it.
// The zero value is ready to use and is safe for concurrent use.
type Mock struct {
	mu   sync.Mutex
	sent []Message
	err  error
}

var _ Mailer = (*Mock)(nil)

// Send validates and records message. It returns the error set with FailWith,
// without recording, to simulate a mail server outage.
func (m *Mock) Send(_ context.Context, message Message) error {
	if _, err := message.validate(); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return m.err
	}
	m.sent = append(m.sent, message)
	return nil
}

// Sent returns a copy of the recorded messages, oldest first.
func (m *Mock) Sent() []Message {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]Message(nil), m.sent...)
}

// FailWith makes every later Send return err. Pass nil to recover.
func (m *Mock) FailWith(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.err = err
}

// Reset forgets recorded messages and any configured failure.
func (m *Mock) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent, m.err = nil, nil
}
