package live

import (
	"encoding/json"
	"time"
)

// Update is a parcel status change as clients see it: public tracking fields
// only. It deliberately has no room for names, addresses, phone numbers, notes,
// merchant or customer ids (API v1.1 section 9.3).
type Update struct {
	TrackingNumber string
	PreviousStatus string
	Status         string
	UpdatedAt      time.Time
}

type updateData struct {
	TrackingNumber string `json:"trackingNumber"`
	PreviousStatus string `json:"previousStatus"`
	Status         string `json:"status"`
	UpdatedAt      string `json:"updatedAt"`
}

// Message encodes the update as a parcel.status.updated server message.
func (u Update) Message() []byte {
	return encode(struct {
		Type string     `json:"type"`
		Data updateData `json:"data"`
	}{
		Type: "parcel.status.updated",
		Data: updateData{
			TrackingNumber: u.TrackingNumber,
			PreviousStatus: u.PreviousStatus,
			Status:         u.Status,
			UpdatedAt:      u.UpdatedAt.UTC().Format(time.RFC3339),
		},
	})
}

// AssignmentUpdate is a courier assignment change as dispatch clients see it.
// It deliberately carries only the public tracking number and the event time:
// no courier or assigner identity, no contact details and no credentials.
type AssignmentUpdate struct {
	TrackingNumber string
	UpdatedAt      time.Time
}

type assignmentData struct {
	TrackingNumber string `json:"trackingNumber"`
	UpdatedAt      string `json:"updatedAt"`
}

// Message encodes the update as a parcel.assigned.updated server message.
// Dispatch screens treat it as a signal to refetch; the REST response remains
// the source of truth for who is assigned.
func (u AssignmentUpdate) Message() []byte {
	return encode(struct {
		Type string         `json:"type"`
		Data assignmentData `json:"data"`
	}{
		Type: "parcel.assigned.updated",
		Data: assignmentData{
			TrackingNumber: u.TrackingNumber,
			UpdatedAt:      u.UpdatedAt.UTC().Format(time.RFC3339),
		},
	})
}

// SubscribedMessage confirms a subscribe message. The web client fetches the
// current state when it receives this, so it must be sent only after the
// subscription is in place.
func SubscribedMessage(message ClientMessage) []byte {
	if message.All {
		return encode(struct {
			Type  string `json:"type"`
			Scope string `json:"scope"`
		}{Type: "subscribed", Scope: "all"})
	}
	return encode(struct {
		Type           string `json:"type"`
		TrackingNumber string `json:"trackingNumber"`
	}{Type: "subscribed", TrackingNumber: message.TrackingNumber})
}

// ErrorMessage encodes an error server message. The text is ours, never the
// client's input or an internal error.
func ErrorMessage(code, text string) []byte {
	return encode(struct {
		Type    string `json:"type"`
		Code    string `json:"code"`
		Message string `json:"message"`
	}{Type: "error", Code: code, Message: text})
}

func encode(value any) []byte {
	data, err := json.Marshal(value)
	if err != nil {
		// These shapes hold only strings, so this cannot happen.
		return []byte(`{"type":"error","code":"INTERNAL_ERROR","message":"internal error"}`)
	}
	return data
}
