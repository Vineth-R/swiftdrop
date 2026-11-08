package live

import (
	"errors"
	"strings"
	"testing"
	"time"
)

const trackingNumber = "SDK7M2Q9XP4"

func TestValidTrackingNumber(t *testing.T) {
	valid := []string{"SDK7M2Q9XP4", "SDN4T8W2LC6", "SDK00000000"}
	for _, value := range valid {
		if !ValidTrackingNumber(value) {
			t.Errorf("ValidTrackingNumber(%q) = false, want true", value)
		}
	}
	invalid := []string{"", "sdk7m2q9xp4", "SDX7M2Q9XP4", "SDK7M2Q9XP", "SDK7M2Q9XP44", " SDK7M2Q9XP4", "SDK7M2Q9XP4\n", "SDK7M2Q9XP-"}
	for _, value := range invalid {
		if ValidTrackingNumber(value) {
			t.Errorf("ValidTrackingNumber(%q) = true, want false", value)
		}
	}
}

func TestValidStatus(t *testing.T) {
	for _, status := range []string{"CREATED", "PICKED_UP", "IN_TRANSIT", "OUT_FOR_DELIVERY", "DELIVERED", "FAILED"} {
		if !ValidStatus(status) {
			t.Errorf("ValidStatus(%q) = false", status)
		}
	}
	for _, status := range []string{"", "LOST", "delivered", "ASSIGNED"} {
		if ValidStatus(status) {
			t.Errorf("ValidStatus(%q) = true", status)
		}
	}
}

func TestParseClientMessage(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want ClientMessage
	}{
		{"ping", `{"type":"ping"}`, ClientMessage{Action: ActionPing}},
		{"subscribe to a parcel", `{"type":"subscribe","trackingNumber":"SDK7M2Q9XP4"}`, ClientMessage{Action: ActionSubscribe, TrackingNumber: trackingNumber}},
		{"unsubscribe from a parcel", `{"type":"unsubscribe","trackingNumber":"SDK7M2Q9XP4"}`, ClientMessage{Action: ActionUnsubscribe, TrackingNumber: trackingNumber}},
		{"subscribe to all", `{"type":"subscribe","scope":"all"}`, ClientMessage{Action: ActionSubscribe, All: true}},
		{"unsubscribe from all", `{"type":"unsubscribe","scope":"all"}`, ClientMessage{Action: ActionUnsubscribe, All: true}},
		{"extra fields are ignored", `{"type":"ping","x":1}`, ClientMessage{Action: ActionPing}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := ParseClientMessage([]byte(test.in))
			if err != nil {
				t.Fatalf("ParseClientMessage() error = %v", err)
			}
			if got != test.want {
				t.Fatalf("ParseClientMessage() = %+v, want %+v", got, test.want)
			}
		})
	}
}

func TestParseClientMessageRejects(t *testing.T) {
	tests := []struct {
		name string
		in   string
	}{
		{"empty", ``},
		{"not json", `not json`},
		{"json array", `[]`},
		{"json string", `"subscribe"`},
		{"no type", `{}`},
		{"unknown type", `{"type":"shout"}`},
		{"subscribe without a target", `{"type":"subscribe"}`},
		{"unsubscribe without a target", `{"type":"unsubscribe"}`},
		{"bad tracking number", `{"type":"subscribe","trackingNumber":"nope"}`},
		{"lowercase tracking number", `{"type":"subscribe","trackingNumber":"sdk7m2q9xp4"}`},
		{"tracking number is not a string", `{"type":"subscribe","trackingNumber":42}`},
		{"scope is not all", `{"type":"subscribe","scope":"mine"}`},
		{"scope is not a string", `{"type":"subscribe","scope":true}`},
		{"both a parcel and a scope", `{"type":"subscribe","trackingNumber":"SDK7M2Q9XP4","scope":"all"}`},
		{"null tracking number", `{"type":"subscribe","trackingNumber":null}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := ParseClientMessage([]byte(test.in)); !errors.Is(err, ErrInvalidMessage) {
				t.Fatalf("ParseClientMessage(%q) error = %v, want ErrInvalidMessage", test.in, err)
			}
		})
	}
}

func TestAuthorize(t *testing.T) {
	anonymous := Principal{}
	customer := Principal{Authenticated: true}
	dispatcher := Principal{Authenticated: true, CanFollowAll: true}

	one := ClientMessage{Action: ActionSubscribe, TrackingNumber: trackingNumber}
	all := ClientMessage{Action: ActionSubscribe, All: true}

	tests := []struct {
		name      string
		principal Principal
		message   ClientMessage
		want      error
	}{
		{"anonymous follows one parcel", anonymous, one, nil},
		{"customer follows one parcel", customer, one, nil},
		{"dispatcher follows one parcel", dispatcher, one, nil},
		{"anonymous cannot follow everything", anonymous, all, ErrUnauthenticated},
		{"customer cannot follow everything", customer, all, ErrForbidden},
		{"dispatcher follows everything", dispatcher, all, nil},
		{"anyone may unsubscribe from everything", anonymous, ClientMessage{Action: ActionUnsubscribe, All: true}, nil},
		{"anyone may ping", anonymous, ClientMessage{Action: ActionPing}, nil},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := Authorize(test.principal, test.message); !errors.Is(got, test.want) {
				t.Fatalf("Authorize() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestMessageLimiter(t *testing.T) {
	var limiter MessageLimiter
	start := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)

	for i := 0; i < MaxMessagesPerWindow; i++ {
		if !limiter.Allow(start.Add(time.Duration(i) * time.Millisecond)) {
			t.Fatalf("message %d should be allowed", i+1)
		}
	}
	if limiter.Allow(start.Add(time.Second)) {
		t.Fatal("the message over the limit should be refused")
	}
	if !limiter.Allow(start.Add(MessageWindow)) {
		t.Fatal("a new window should start counting again")
	}
}

func TestLimitsMatchTheContract(t *testing.T) {
	if MaxSubscriptions != 20 {
		t.Errorf("MaxSubscriptions = %d, want 20", MaxSubscriptions)
	}
	if PingInterval != 30*time.Second || IdleTimeout != 60*time.Second {
		t.Errorf("PingInterval = %s, IdleTimeout = %s", PingInterval, IdleTimeout)
	}
	if CloseTokenInvalid != 4401 || CloseForbidden != 4403 || CloseTooMany != 4429 {
		t.Errorf("close codes = %d, %d, %d", CloseTokenInvalid, CloseForbidden, CloseTooMany)
	}
	// The longest valid message must fit with room to spare.
	longest := `{"type":"unsubscribe","trackingNumber":"` + strings.Repeat("A", 11) + `"}`
	if len(longest) > MaxMessageBytes {
		t.Errorf("a valid message of %d bytes does not fit in %d", len(longest), MaxMessageBytes)
	}
}
