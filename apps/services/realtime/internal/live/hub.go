package live

import "sync"

// Subscriber is one connection as the hub sees it.
type Subscriber interface {
	// Deliver queues an update for the connection without blocking. It returns
	// false when the connection cannot keep up.
	Deliver(Update) bool
	// DeliverAssignment queues an assignment notice for the connection without
	// blocking. It returns false when the connection cannot keep up.
	DeliverAssignment(AssignmentUpdate) bool
	// Overflow is called once, after the hub has removed the subscriber, when
	// Deliver or DeliverAssignment returned false. The connection should be
	// closed; the client will reconnect and fetch the current state again.
	Overflow()
}

// Hub tracks which connections follow which parcels and routes each update to
// them. It is safe for concurrent use.
type Hub struct {
	mu         sync.RWMutex
	byTracking map[string]map[Subscriber]struct{}
	followAll  map[Subscriber]struct{}
	members    map[Subscriber]*subscriptions
}

// subscriptions is what one connection follows.
type subscriptions struct {
	tracking map[string]struct{}
	all      bool
}

func (s *subscriptions) count() int {
	if s.all {
		return len(s.tracking) + 1
	}
	return len(s.tracking)
}

// NewHub returns an empty hub.
func NewHub() *Hub {
	return &Hub{
		byTracking: make(map[string]map[Subscriber]struct{}),
		followAll:  make(map[Subscriber]struct{}),
		members:    make(map[Subscriber]*subscriptions),
	}
}

// Apply carries out a subscribe or unsubscribe for subscriber. Subscribing to
// something already followed changes nothing and does not count twice.
// Subscribing past MaxSubscriptions returns ErrTooManySubscriptions and
// changes nothing. Other actions are ignored.
func (h *Hub) Apply(subscriber Subscriber, message ClientMessage) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	subs := h.members[subscriber]

	switch message.Action {
	case ActionSubscribe:
		if subs == nil {
			subs = &subscriptions{tracking: make(map[string]struct{})}
		}
		if message.All {
			if subs.all {
				return nil
			}
			if subs.count() >= MaxSubscriptions {
				return ErrTooManySubscriptions
			}
			subs.all = true
			h.followAll[subscriber] = struct{}{}
		} else {
			if _, followed := subs.tracking[message.TrackingNumber]; followed {
				return nil
			}
			if subs.count() >= MaxSubscriptions {
				return ErrTooManySubscriptions
			}
			subs.tracking[message.TrackingNumber] = struct{}{}
			group := h.byTracking[message.TrackingNumber]
			if group == nil {
				group = make(map[Subscriber]struct{})
				h.byTracking[message.TrackingNumber] = group
			}
			group[subscriber] = struct{}{}
		}
		h.members[subscriber] = subs
	case ActionUnsubscribe:
		if subs == nil {
			return nil
		}
		if message.All {
			if subs.all {
				subs.all = false
				delete(h.followAll, subscriber)
			}
		} else if _, followed := subs.tracking[message.TrackingNumber]; followed {
			delete(subs.tracking, message.TrackingNumber)
			h.dropFromGroup(message.TrackingNumber, subscriber)
		}
	}
	return nil
}

// Remove drops every subscription of subscriber. Call it when the connection
// closes. It is safe to call more than once.
func (h *Hub) Remove(subscriber Subscriber) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.remove(subscriber)
}

func (h *Hub) remove(subscriber Subscriber) {
	subs := h.members[subscriber]
	if subs == nil {
		return
	}
	for trackingNumber := range subs.tracking {
		h.dropFromGroup(trackingNumber, subscriber)
	}
	delete(h.followAll, subscriber)
	delete(h.members, subscriber)
}

func (h *Hub) dropFromGroup(trackingNumber string, subscriber Subscriber) {
	group := h.byTracking[trackingNumber]
	delete(group, subscriber)
	if len(group) == 0 {
		delete(h.byTracking, trackingNumber)
	}
}

// Broadcast sends update to every connection that follows its parcel and to
// every connection that follows everything, each at most once. It never blocks:
// a connection that cannot keep up is removed and told to close. It returns the
// number of connections the update was queued for.
func (h *Hub) Broadcast(update Update) int {
	h.mu.RLock()
	delivered := 0
	var overflowed []Subscriber
	seen := make(map[Subscriber]struct{})
	deliver := func(subscriber Subscriber) {
		if _, done := seen[subscriber]; done {
			return
		}
		seen[subscriber] = struct{}{}
		if subscriber.Deliver(update) {
			delivered++
			return
		}
		overflowed = append(overflowed, subscriber)
	}
	for subscriber := range h.byTracking[update.TrackingNumber] {
		deliver(subscriber)
	}
	for subscriber := range h.followAll {
		deliver(subscriber)
	}
	h.mu.RUnlock()

	h.dropOverflowed(overflowed)
	return delivered
}

// BroadcastAssignment sends an assignment notice to every connection that
// follows everything (dispatch and admin), each at most once. It never sends to
// a per-parcel public subscriber: assignment is an operational event. It never
// blocks: a connection that cannot keep up is removed and told to close. It
// returns the number of connections the notice was queued for.
func (h *Hub) BroadcastAssignment(update AssignmentUpdate) int {
	h.mu.RLock()
	delivered := 0
	var overflowed []Subscriber
	for subscriber := range h.followAll {
		if subscriber.DeliverAssignment(update) {
			delivered++
			continue
		}
		overflowed = append(overflowed, subscriber)
	}
	h.mu.RUnlock()

	h.dropOverflowed(overflowed)
	return delivered
}

// dropOverflowed removes subscribers that could not keep up and tells them to
// close. It is called after the read lock is released.
func (h *Hub) dropOverflowed(overflowed []Subscriber) {
	if len(overflowed) == 0 {
		return
	}
	h.mu.Lock()
	for _, subscriber := range overflowed {
		h.remove(subscriber)
	}
	h.mu.Unlock()
	for _, subscriber := range overflowed {
		subscriber.Overflow()
	}
}

// Connections returns how many connections hold at least one subscription.
func (h *Hub) Connections() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.members)
}
