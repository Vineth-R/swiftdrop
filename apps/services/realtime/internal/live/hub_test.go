package live

import (
	"errors"
	"fmt"
	"sync"
	"testing"
)

type fakeSubscriber struct {
	mu        sync.Mutex
	updates   []Update
	capacity  int
	overflows int
}

func newSubscriber(capacity int) *fakeSubscriber {
	return &fakeSubscriber{capacity: capacity}
}

func (f *fakeSubscriber) Deliver(update Update) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.updates) >= f.capacity {
		return false
	}
	f.updates = append(f.updates, update)
	return true
}

func (f *fakeSubscriber) Overflow() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.overflows++
}

func (f *fakeSubscriber) received() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.updates)
}

func follow(number string) ClientMessage {
	return ClientMessage{Action: ActionSubscribe, TrackingNumber: number}
}

func unfollow(number string) ClientMessage {
	return ClientMessage{Action: ActionUnsubscribe, TrackingNumber: number}
}

func updateFor(number string) Update {
	return Update{TrackingNumber: number, PreviousStatus: "CREATED", Status: "PICKED_UP"}
}

func TestBroadcastReachesOnlyFollowers(t *testing.T) {
	hub := NewHub()
	follower := newSubscriber(10)
	other := newSubscriber(10)
	stranger := newSubscriber(10)
	hub.Apply(follower, follow(trackingNumber))
	hub.Apply(other, follow("SDN4T8W2LC6"))

	if got := hub.Broadcast(updateFor(trackingNumber)); got != 1 {
		t.Fatalf("Broadcast() = %d, want 1", got)
	}
	if follower.received() != 1 || other.received() != 0 || stranger.received() != 0 {
		t.Fatalf("received = %d, %d, %d", follower.received(), other.received(), stranger.received())
	}
}

func TestFollowAllReceivesEveryParcel(t *testing.T) {
	hub := NewHub()
	dispatcher := newSubscriber(10)
	hub.Apply(dispatcher, ClientMessage{Action: ActionSubscribe, All: true})

	hub.Broadcast(updateFor(trackingNumber))
	hub.Broadcast(updateFor("SDN4T8W2LC6"))
	if dispatcher.received() != 2 {
		t.Fatalf("received = %d, want 2", dispatcher.received())
	}
}

func TestBroadcastDeliversOnceWhenFollowingBothWays(t *testing.T) {
	hub := NewHub()
	both := newSubscriber(10)
	hub.Apply(both, follow(trackingNumber))
	hub.Apply(both, ClientMessage{Action: ActionSubscribe, All: true})

	if got := hub.Broadcast(updateFor(trackingNumber)); got != 1 {
		t.Fatalf("Broadcast() = %d, want 1", got)
	}
	if both.received() != 1 {
		t.Fatalf("received = %d, want 1", both.received())
	}
}

func TestUnsubscribeStopsUpdates(t *testing.T) {
	hub := NewHub()
	subscriber := newSubscriber(10)
	hub.Apply(subscriber, follow(trackingNumber))
	hub.Apply(subscriber, unfollow(trackingNumber))

	hub.Broadcast(updateFor(trackingNumber))
	if subscriber.received() != 0 {
		t.Fatalf("received = %d, want 0", subscriber.received())
	}

	all := newSubscriber(10)
	hub.Apply(all, ClientMessage{Action: ActionSubscribe, All: true})
	hub.Apply(all, ClientMessage{Action: ActionUnsubscribe, All: true})
	hub.Broadcast(updateFor(trackingNumber))
	if all.received() != 0 {
		t.Fatalf("received after unsubscribing from all = %d, want 0", all.received())
	}
}

func TestUnsubscribeFromSomethingNotFollowedIsHarmless(t *testing.T) {
	hub := NewHub()
	subscriber := newSubscriber(10)
	if err := hub.Apply(subscriber, unfollow(trackingNumber)); err != nil {
		t.Fatalf("Apply() = %v", err)
	}
	if err := hub.Apply(subscriber, ClientMessage{Action: ActionUnsubscribe, All: true}); err != nil {
		t.Fatalf("Apply() = %v", err)
	}
}

func TestSubscribingTwiceCountsOnce(t *testing.T) {
	hub := NewHub()
	subscriber := newSubscriber(10)
	for i := 0; i < MaxSubscriptions+5; i++ {
		if err := hub.Apply(subscriber, follow(trackingNumber)); err != nil {
			t.Fatalf("Apply() #%d = %v", i, err)
		}
	}
	hub.Broadcast(updateFor(trackingNumber))
	if subscriber.received() != 1 {
		t.Fatalf("received = %d, want 1", subscriber.received())
	}
}

func TestSubscriptionLimit(t *testing.T) {
	hub := NewHub()
	subscriber := newSubscriber(10)
	for i := 0; i < MaxSubscriptions; i++ {
		number := fmt.Sprintf("SDK%08d", i)
		if err := hub.Apply(subscriber, follow(number)); err != nil {
			t.Fatalf("subscription %d refused: %v", i+1, err)
		}
	}

	err := hub.Apply(subscriber, follow("SDKZZZZZZZZ"))
	if !errors.Is(err, ErrTooManySubscriptions) {
		t.Fatalf("Apply() over the limit = %v, want ErrTooManySubscriptions", err)
	}
	// The refused subscription must not have been recorded.
	hub.Broadcast(updateFor("SDKZZZZZZZZ"))
	if subscriber.received() != 0 {
		t.Fatalf("received = %d, want 0", subscriber.received())
	}

	// Following everything counts as one subscription too.
	if err := hub.Apply(subscriber, ClientMessage{Action: ActionSubscribe, All: true}); !errors.Is(err, ErrTooManySubscriptions) {
		t.Fatalf("scope all over the limit = %v, want ErrTooManySubscriptions", err)
	}

	// Leaving one makes room again.
	hub.Apply(subscriber, unfollow("SDK00000000"))
	if err := hub.Apply(subscriber, follow("SDKZZZZZZZZ")); err != nil {
		t.Fatalf("Apply() after making room = %v", err)
	}
}

func TestFollowAllCountsAsOneSubscription(t *testing.T) {
	hub := NewHub()
	subscriber := newSubscriber(10)
	hub.Apply(subscriber, ClientMessage{Action: ActionSubscribe, All: true})
	for i := 0; i < MaxSubscriptions-1; i++ {
		if err := hub.Apply(subscriber, follow(fmt.Sprintf("SDK%08d", i))); err != nil {
			t.Fatalf("subscription %d refused: %v", i+2, err)
		}
	}
	if err := hub.Apply(subscriber, follow("SDKZZZZZZZZ")); !errors.Is(err, ErrTooManySubscriptions) {
		t.Fatalf("Apply() = %v, want ErrTooManySubscriptions", err)
	}
}

func TestRemoveDropsEverySubscription(t *testing.T) {
	hub := NewHub()
	subscriber := newSubscriber(10)
	hub.Apply(subscriber, follow(trackingNumber))
	hub.Apply(subscriber, ClientMessage{Action: ActionSubscribe, All: true})
	if hub.Connections() != 1 {
		t.Fatalf("Connections() = %d, want 1", hub.Connections())
	}

	hub.Remove(subscriber)
	hub.Remove(subscriber) // twice is fine
	hub.Broadcast(updateFor(trackingNumber))
	if subscriber.received() != 0 || hub.Connections() != 0 {
		t.Fatalf("received = %d, Connections() = %d", subscriber.received(), hub.Connections())
	}
	if len(hub.byTracking) != 0 || len(hub.followAll) != 0 {
		t.Fatalf("hub kept state after Remove: %d groups, %d followers", len(hub.byTracking), len(hub.followAll))
	}
}

func TestSlowSubscriberIsRemovedAndToldToClose(t *testing.T) {
	hub := NewHub()
	slow := newSubscriber(1)
	healthy := newSubscriber(10)
	hub.Apply(slow, follow(trackingNumber))
	hub.Apply(healthy, follow(trackingNumber))

	hub.Broadcast(updateFor(trackingNumber)) // fills the slow one's buffer
	if got := hub.Broadcast(updateFor(trackingNumber)); got != 1 {
		t.Fatalf("Broadcast() = %d, want 1 (only the healthy subscriber)", got)
	}
	if slow.overflows != 1 {
		t.Fatalf("overflows = %d, want 1", slow.overflows)
	}
	if healthy.received() != 2 {
		t.Fatalf("healthy received = %d, want 2", healthy.received())
	}

	// The slow subscriber is gone: it is not asked again and not told twice.
	hub.Broadcast(updateFor(trackingNumber))
	if slow.overflows != 1 || slow.received() != 1 {
		t.Fatalf("slow overflows = %d, received = %d", slow.overflows, slow.received())
	}
}

func TestBroadcastWithNoFollowers(t *testing.T) {
	if got := NewHub().Broadcast(updateFor(trackingNumber)); got != 0 {
		t.Fatalf("Broadcast() = %d, want 0", got)
	}
}

func TestHubIsSafeForConcurrentUse(t *testing.T) {
	hub := NewHub()
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			subscriber := newSubscriber(1000)
			for j := 0; j < 50; j++ {
				hub.Apply(subscriber, follow(trackingNumber))
				hub.Broadcast(updateFor(trackingNumber))
				hub.Apply(subscriber, unfollow(trackingNumber))
			}
			hub.Remove(subscriber)
		}(i)
	}
	wg.Wait()
	if hub.Connections() != 0 {
		t.Fatalf("Connections() = %d, want 0", hub.Connections())
	}
}
