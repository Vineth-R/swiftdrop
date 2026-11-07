// Package live holds the rules of the realtime service that do not depend on
// the WebSocket library: what a client may send, who may follow what, the
// limits, and the hub that routes status updates to the right connections.
// API v1.1 section 9 is the contract.
package live

import (
	"encoding/json"
	"errors"
	"regexp"
	"time"
)

// Limits from API v1.1 section 9.1.
const (
	// MaxSubscriptions is the number of subscriptions one connection may hold.
	// Following everything (scope all) counts as one.
	MaxSubscriptions = 20
	// PingInterval is how often the server pings an open connection.
	PingInterval = 30 * time.Second
	// IdleTimeout closes a connection that has shown no sign of life, no pong
	// and no message, for this long.
	IdleTimeout = 60 * time.Second
	// MaxMessageBytes is the largest client message accepted. The longest valid
	// message is a short JSON object.
	MaxMessageBytes = 1024
	// MaxMessagesPerWindow client messages are allowed in each MessageWindow.
	// The web client sends a ping every 25 seconds, so this is far above normal
	// use and only stops a runaway client.
	MaxMessagesPerWindow = 60
	MessageWindow        = 10 * time.Second
)

// Close codes from API v1.1 section 9.4.
const (
	CloseNormal       = 1000
	CloseGoingAway    = 1001
	CloseTokenInvalid = 4401
	CloseForbidden    = 4403
	CloseTooMany      = 4429
)

var (
	// ErrInvalidMessage is returned for a message the service does not accept.
	ErrInvalidMessage = errors.New("invalid message")
	// ErrUnauthenticated means the caller must sign in (or renew the session)
	// before following everything.
	ErrUnauthenticated = errors.New("authentication required")
	// ErrForbidden means the caller is signed in but may not do this.
	ErrForbidden = errors.New("not allowed")
	// ErrTooManySubscriptions means the connection is at MaxSubscriptions.
	ErrTooManySubscriptions = errors.New("too many subscriptions")
)

// trackingNumberPattern matches the tracking numbers the parcel service issues:
// SD, K or N, then eight letters or digits.
var trackingNumberPattern = regexp.MustCompile(`^SD[KN][A-Z0-9]{8}$`)

// ValidTrackingNumber reports whether value looks like a SwiftDrop tracking
// number.
func ValidTrackingNumber(value string) bool {
	return trackingNumberPattern.MatchString(value)
}

var parcelStatuses = map[string]struct{}{
	"CREATED":          {},
	"PICKED_UP":        {},
	"IN_TRANSIT":       {},
	"OUT_FOR_DELIVERY": {},
	"DELIVERED":        {},
	"FAILED":           {},
}

// ValidStatus reports whether value is a parcel status.
func ValidStatus(value string) bool {
	_, ok := parcelStatuses[value]
	return ok
}

// Principal is who is on the other end of a connection.
type Principal struct {
	// Authenticated is true when a valid access token was presented.
	Authenticated bool
	// CanFollowAll is true for DISPATCHER and ADMIN.
	CanFollowAll bool
}

// Action is what a client message asks for.
type Action int

const (
	ActionSubscribe Action = iota + 1
	ActionUnsubscribe
	ActionPing
)

// ClientMessage is a validated message from a client (API v1.1 section 9.2).
type ClientMessage struct {
	Action Action
	// TrackingNumber is set when the message is about one parcel.
	TrackingNumber string
	// All is true for scope "all".
	All bool
}

type clientEnvelope struct {
	Type           string  `json:"type"`
	TrackingNumber *string `json:"trackingNumber"`
	Scope          *string `json:"scope"`
}

// ParseClientMessage validates one text message from a client. It never trusts
// the content: the type must be known, and a subscribe or unsubscribe names
// exactly one target, either a well-formed tracking number or scope "all".
func ParseClientMessage(data []byte) (ClientMessage, error) {
	var envelope clientEnvelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		return ClientMessage{}, ErrInvalidMessage
	}

	switch envelope.Type {
	case "ping":
		return ClientMessage{Action: ActionPing}, nil
	case "subscribe", "unsubscribe":
		message := ClientMessage{Action: ActionSubscribe}
		if envelope.Type == "unsubscribe" {
			message.Action = ActionUnsubscribe
		}
		hasNumber := envelope.TrackingNumber != nil
		hasScope := envelope.Scope != nil
		switch {
		case hasNumber && !hasScope && ValidTrackingNumber(*envelope.TrackingNumber):
			message.TrackingNumber = *envelope.TrackingNumber
			return message, nil
		case hasScope && !hasNumber && *envelope.Scope == "all":
			message.All = true
			return message, nil
		}
	}
	return ClientMessage{}, ErrInvalidMessage
}

// Authorize decides whether principal may send message. Following one parcel
// is open to everyone, including anonymous visitors on the public tracking
// page. Following everything is for DISPATCHER and ADMIN: an anonymous caller
// gets ErrUnauthenticated (the web client then renews its session and tries
// again) and a signed-in caller without the role gets ErrForbidden.
func Authorize(principal Principal, message ClientMessage) error {
	if message.Action != ActionSubscribe || !message.All {
		return nil
	}
	if !principal.Authenticated {
		return ErrUnauthenticated
	}
	if !principal.CanFollowAll {
		return ErrForbidden
	}
	return nil
}

// MessageLimiter allows at most MaxMessagesPerWindow messages in each
// MessageWindow, counted in fixed windows. It is not safe for concurrent use;
// one connection's reader owns it.
type MessageLimiter struct {
	start time.Time
	count int
}

// Allow records one message at now and reports whether it is within the limit.
func (l *MessageLimiter) Allow(now time.Time) bool {
	if l.start.IsZero() || now.Sub(l.start) >= MessageWindow {
		l.start = now
		l.count = 0
	}
	l.count++
	return l.count <= MaxMessagesPerWindow
}
