// Package conversation provides a bounded, expiring, in-process conversation
// store. It is intentionally not durable and not shared between replicas; the
// MVP documents this limitation rather than adding Redis or a database.
package conversation

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"regexp"
	"strings"
	"sync"
	"time"
)

var (
	// ErrInvalidID means the conversation ID does not match the public format.
	ErrInvalidID = errors.New("invalid conversation id")
	// ErrNotFound means the conversation is unknown, expired or evicted.
	ErrNotFound = errors.New("conversation not found")
)

// Roles stored in a conversation. Tool turns are not persisted.
const (
	RoleUser      = "user"
	RoleAssistant = "assistant"
)

var (
	idPattern      = regexp.MustCompile(`^cv[0-9a-f]{32}$`)
	guestIDPattern = regexp.MustCompile(`^gs[0-9a-f]{32}$`)
)

// Message is one persisted text turn.
type Message struct {
	Role string
	Text string
}

// Conversation is a snapshot returned by the store. Callers must not mutate
// the Messages slice expecting the store to change.
type Conversation struct {
	ID        string
	OwnerID   string
	Anonymous bool
	// GuestSessionID binds an anonymous conversation to one browser session.
	// It is empty for authenticated conversations and is never exposed.
	GuestSessionID string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	Messages       []Message

	// seq gives eviction a deterministic order even when two conversations
	// share the same clock timestamp.
	seq uint64
}

// Option customises a Store.
type Option func(*Store)

// WithClock replaces the time source for deterministic tests.
func WithClock(now func() time.Time) Option {
	return func(store *Store) {
		if now != nil {
			store.now = now
		}
	}
}

// WithRandom replaces the ID entropy source for deterministic tests.
func WithRandom(reader interface{ Read([]byte) (int, error) }) Option {
	return func(store *Store) {
		if reader != nil {
			store.random = reader
		}
	}
}

// Store is a concurrency-safe bounded conversation store.
type Store struct {
	mu          sync.Mutex
	items       map[string]*Conversation
	max         int
	ttl         time.Duration
	maxMessages int
	now         func() time.Time
	random      interface{ Read([]byte) (int, error) }
	nextSeq     uint64
}

// New returns a store. Values are clamped to safe defaults.
func New(max int, ttl time.Duration, maxMessages int, opts ...Option) *Store {
	if max < 1 {
		max = 1
	}
	if ttl <= 0 {
		ttl = 30 * time.Minute
	}
	if maxMessages < 1 {
		maxMessages = 40
	}
	store := &Store{
		items:       make(map[string]*Conversation),
		max:         max,
		ttl:         ttl,
		maxMessages: maxMessages,
		now:         time.Now,
		random:      rand.Reader,
	}
	for _, opt := range opts {
		opt(store)
	}
	return store
}

// ValidID reports whether id has the public conversation ID format.
func ValidID(id string) bool {
	return idPattern.MatchString(id)
}

// ValidGuestSessionID reports whether id has the server-generated guest
// session format.
func ValidGuestSessionID(id string) bool {
	return guestIDPattern.MatchString(id)
}

// Create starts a new conversation. ownerID must be empty for anonymous
// conversations and non-empty for authenticated ones. Anonymous conversations
// require the server-generated guest session ID that owns them.
func (s *Store) Create(ownerID, guestSessionID string, anonymous bool) (Conversation, error) {
	if anonymous {
		if ownerID != "" {
			return Conversation{}, errors.New("anonymous conversation must not have an owner")
		}
		if !ValidGuestSessionID(guestSessionID) {
			return Conversation{}, errors.New("anonymous conversation requires a valid guest session")
		}
	} else {
		if strings.TrimSpace(ownerID) == "" {
			return Conversation{}, errors.New("authenticated conversation requires an owner")
		}
		if guestSessionID != "" {
			return Conversation{}, errors.New("authenticated conversation must not have a guest session")
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.now().UTC()
	s.evict(now)

	id, err := s.newID()
	if err != nil {
		return Conversation{}, err
	}

	s.nextSeq++
	item := &Conversation{
		ID:             id,
		OwnerID:        ownerID,
		Anonymous:      anonymous,
		GuestSessionID: guestSessionID,
		CreatedAt:      now,
		UpdatedAt:      now,
		Messages:       []Message{},
		seq:            s.nextSeq,
	}
	s.items[id] = item
	return clone(*item), nil
}

// Load returns a snapshot of a live conversation.
func (s *Store) Load(id string) (Conversation, error) {
	if !ValidID(id) {
		return Conversation{}, ErrInvalidID
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	item, ok := s.items[id]
	if !ok {
		return Conversation{}, ErrNotFound
	}
	if s.expired(item, s.now().UTC()) {
		delete(s.items, id)
		return Conversation{}, ErrNotFound
	}
	return clone(*item), nil
}

// Append adds text turns and trims the stored history. It refreshes expiry.
func (s *Store) Append(id string, messages ...Message) error {
	if !ValidID(id) {
		return ErrInvalidID
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	item, ok := s.items[id]
	if !ok {
		return ErrNotFound
	}
	now := s.now().UTC()
	if s.expired(item, now) {
		delete(s.items, id)
		return ErrNotFound
	}

	for _, message := range messages {
		if strings.TrimSpace(message.Text) == "" {
			continue
		}
		item.Messages = append(item.Messages, message)
	}
	if len(item.Messages) > s.maxMessages {
		item.Messages = append([]Message(nil), item.Messages[len(item.Messages)-s.maxMessages:]...)
	}
	item.UpdatedAt = now
	return nil
}

func (s *Store) newID() (string, error) {
	for attempt := 0; attempt < 5; attempt++ {
		var buffer [16]byte
		if _, err := s.random.Read(buffer[:]); err != nil {
			return "", err
		}
		id := "cv" + hex.EncodeToString(buffer[:])
		if _, exists := s.items[id]; !exists {
			return id, nil
		}
	}
	return "", errors.New("could not allocate a conversation id")
}

func (s *Store) expired(item *Conversation, now time.Time) bool {
	return now.Sub(item.UpdatedAt) > s.ttl
}

// evict removes expired entries, then the oldest entry when the store is full.
func (s *Store) evict(now time.Time) {
	for id, item := range s.items {
		if s.expired(item, now) {
			delete(s.items, id)
		}
	}
	if len(s.items) < s.max {
		return
	}
	var oldestID string
	var oldestSeq uint64
	for id, item := range s.items {
		if oldestID == "" || item.seq < oldestSeq {
			oldestID = id
			oldestSeq = item.seq
		}
	}
	if oldestID != "" {
		delete(s.items, oldestID)
	}
}

func clone(item Conversation) Conversation {
	item.Messages = append([]Message(nil), item.Messages...)
	return item
}
