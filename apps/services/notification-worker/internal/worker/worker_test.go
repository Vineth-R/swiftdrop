package worker

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/Vineth-R/swiftdrop/packages/go-common/messaging"
)

func newHandler() *Handler {
	return New(slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func TestHandleValidEvent(t *testing.T) {
	envelope := messaging.Envelope{
		ID:         "event-1",
		Type:       EventType,
		Version:    1,
		OccurredAt: time.Now().UTC(),
		Producer:   "parcel",
		Data:       json.RawMessage(`{"recipient":"customer@example.com"}`),
	}
	body, err := json.Marshal(envelope)
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}

	if err := newHandler().Handle(context.Background(), messaging.Message{Body: body}); err != nil {
		t.Fatalf("Handle() returned error: %v", err)
	}
}

func TestHandleRejectsInvalidMessages(t *testing.T) {
	validData := json.RawMessage(`{"recipient":"customer@example.com"}`)

	tests := map[string]messaging.Envelope{
		"wrong event type": {
			ID:         "event-1",
			Type:       "parcel.created.v1",
			Version:    1,
			OccurredAt: time.Now().UTC(),
			Producer:   "parcel",
			Data:       validData,
		},
		"wrong event version": {
			ID:         "event-1",
			Type:       EventType,
			Version:    2,
			OccurredAt: time.Now().UTC(),
			Producer:   "parcel",
			Data:       validData,
		},
		"invalid envelope": {
			ID:         "event-1",
			Type:       EventType,
			Version:    1,
			OccurredAt: time.Now().UTC(),
			Data:       validData,
		},
	}

	for name, envelope := range tests {
		t.Run(name, func(t *testing.T) {
			body, err := json.Marshal(envelope)
			if err != nil {
				t.Fatalf("marshal envelope: %v", err)
			}
			err = newHandler().Handle(context.Background(), messaging.Message{Body: body})
			if err == nil {
				t.Fatal("Handle() should have failed")
			}
			if !messaging.IsPermanent(err) {
				t.Fatalf("Handle() error is not permanent: %v", err)
			}
		})
	}

	err := newHandler().Handle(context.Background(), messaging.Message{Body: []byte(`{`)})
	if err == nil {
		t.Fatal("Handle() should fail for malformed JSON")
	}
	if !messaging.IsPermanent(err) {
		t.Fatalf("Handle() error is not permanent: %v", err)
	}
}
