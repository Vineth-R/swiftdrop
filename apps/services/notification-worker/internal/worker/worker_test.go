package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/Vineth-R/swiftdrop/packages/go-common/mailer"
	"github.com/Vineth-R/swiftdrop/packages/go-common/messaging"
)

func newHandler() (*Handler, *mailer.Mock) {
	sender := &mailer.Mock{}
	return New(slog.New(slog.NewTextHandler(io.Discard, nil)), sender), sender
}

func envelopeBody(t *testing.T, data string) []byte {
	t.Helper()

	body, err := json.Marshal(messaging.Envelope{
		ID:         "event-1",
		Type:       EventType,
		Version:    1,
		OccurredAt: time.Now().UTC(),
		Producer:   "parcel",
		Data:       json.RawMessage(data),
	})
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}
	return body
}

func TestHandleDeliversEmail(t *testing.T) {
	handler, sender := newHandler()
	body := envelopeBody(t, `{
		"recipient":"customer@example.com",
		"channel":"email",
		"template":"Your parcel has shipped",
		"variables":{"tracking_number":"SD123","status":"IN_TRANSIT"}
	}`)

	if err := handler.Handle(context.Background(), messaging.Message{Body: body}); err != nil {
		t.Fatalf("Handle() returned error: %v", err)
	}

	sent := sender.Sent()
	if len(sent) != 1 {
		t.Fatalf("sent messages = %d, want 1", len(sent))
	}
	if sent[0].To != "customer@example.com" {
		t.Fatalf("To = %q", sent[0].To)
	}
	if sent[0].Subject != "Your parcel has shipped" {
		t.Fatalf("Subject = %q", sent[0].Subject)
	}
	wantText := "Your parcel has shipped\n\nstatus: IN_TRANSIT\ntracking_number: SD123"
	if sent[0].Text != wantText {
		t.Fatalf("Text = %q, want %q", sent[0].Text, wantText)
	}
}

func TestHandleAcceptsTemplateWithoutVariables(t *testing.T) {
	handler, sender := newHandler()
	body := envelopeBody(t, `{"recipient":"customer@example.com","channel":"email","template":"Your parcel has shipped"}`)

	if err := handler.Handle(context.Background(), messaging.Message{Body: body}); err != nil {
		t.Fatalf("Handle() returned error: %v", err)
	}
	if sent := sender.Sent(); len(sent) != 1 || sent[0].Text != "Your parcel has shipped" {
		t.Fatalf("sent = %+v", sent)
	}
}

func TestHandleRejectsInvalidMessages(t *testing.T) {
	validData := `{"recipient":"customer@example.com","channel":"email","template":"Your parcel has shipped"}`

	envelopes := map[string]messaging.Envelope{
		"wrong event type": {
			ID: "event-1", Type: "parcel.created.v1", Version: 1,
			OccurredAt: time.Now().UTC(), Producer: "parcel", Data: json.RawMessage(validData),
		},
		"wrong event version": {
			ID: "event-1", Type: EventType, Version: 2,
			OccurredAt: time.Now().UTC(), Producer: "parcel", Data: json.RawMessage(validData),
		},
		"invalid envelope": {
			ID: "event-1", Type: EventType, Version: 1,
			OccurredAt: time.Now().UTC(), Data: json.RawMessage(validData),
		},
	}

	for name, envelope := range envelopes {
		t.Run(name, func(t *testing.T) {
			handler, sender := newHandler()
			body, err := json.Marshal(envelope)
			if err != nil {
				t.Fatalf("marshal envelope: %v", err)
			}

			err = handler.Handle(context.Background(), messaging.Message{Body: body})
			if err == nil || !messaging.IsPermanent(err) {
				t.Fatalf("Handle() error = %v, want a permanent error", err)
			}
			if len(sender.Sent()) != 0 {
				t.Fatal("an invalid envelope triggered delivery")
			}
		})
	}

	t.Run("malformed envelope JSON", func(t *testing.T) {
		handler, _ := newHandler()
		err := handler.Handle(context.Background(), messaging.Message{Body: []byte(`{`)})
		if err == nil || !messaging.IsPermanent(err) {
			t.Fatalf("Handle() error = %v, want a permanent error", err)
		}
	})

	t.Run("malformed data", func(t *testing.T) {
		handler, sender := newHandler()
		body := []byte(`{"event_id":"event-1","event_type":"notification.requested.v1","event_version":1,"occurred_at":"2026-11-02T09:30:00Z","producer":"parcel","data":{"recipient":}}`)
		err := handler.Handle(context.Background(), messaging.Message{Body: body})
		if err == nil || !messaging.IsPermanent(err) {
			t.Fatalf("Handle() error = %v, want a permanent error", err)
		}
		if len(sender.Sent()) != 0 {
			t.Fatal("malformed data triggered delivery")
		}
	})

	invalidData := map[string]string{
		"missing recipient":          `{"channel":"email","template":"hello"}`,
		"invalid recipient":          `{"recipient":"not-an-email","channel":"email","template":"hello"}`,
		"recipient display name":     `{"recipient":"Customer <customer@example.com>","channel":"email","template":"hello"}`,
		"recipient header injection": `{"recipient":"customer@example.com\r\nBcc: victim@example.com","channel":"email","template":"hello"}`,
		"sms channel":                `{"recipient":"customer@example.com","channel":"sms","template":"hello"}`,
		"unknown channel":            `{"recipient":"customer@example.com","channel":"push","template":"hello"}`,
		"missing template":           `{"recipient":"customer@example.com","channel":"email"}`,
		"blank template":             `{"recipient":"customer@example.com","channel":"email","template":"  "}`,
		"template header injection":  `{"recipient":"customer@example.com","channel":"email","template":"hello\r\nBcc: victim@example.com"}`,
		"overlong template":          `{"recipient":"customer@example.com","channel":"email","template":"` + strings.Repeat("a", mailer.MaxSubjectLength+1) + `"}`,
		"too many variables":         `{"recipient":"customer@example.com","channel":"email","template":"hello","variables":{` + manyVariables(65) + `}}`,
		"overlong variable key":      `{"recipient":"customer@example.com","channel":"email","template":"hello","variables":{"` + strings.Repeat("k", maxVariableKey+1) + `":"v"}}`,
		"overlong variable value":    `{"recipient":"customer@example.com","channel":"email","template":"hello","variables":{"key":"` + strings.Repeat("v", maxVariableVal+1) + `"}}`,
	}

	for name, data := range invalidData {
		t.Run(name, func(t *testing.T) {
			handler, sender := newHandler()
			err := handler.Handle(context.Background(), messaging.Message{Body: envelopeBody(t, data)})
			if err == nil || !messaging.IsPermanent(err) {
				t.Fatalf("Handle() error = %v, want a permanent error", err)
			}
			if len(sender.Sent()) != 0 {
				t.Fatal("invalid data triggered delivery")
			}
		})
	}
}

func manyVariables(count int) string {
	entries := make([]string, 0, count)
	for index := 0; index < count; index++ {
		entries = append(entries, fmt.Sprintf(`"k%d":"v"`, index))
	}
	return strings.Join(entries, ",")
}

type failingSender struct {
	err error
}

func (s failingSender) Send(context.Context, mailer.Message) error {
	return s.err
}

func TestHandleRetryableDeliveryFailure(t *testing.T) {
	var logs bytes.Buffer
	handler := New(
		slog.New(slog.NewJSONHandler(&logs, nil)),
		failingSender{err: errors.New("smtp connection refused")},
	)
	body := envelopeBody(t, `{"recipient":"customer@example.com","channel":"email","template":"Your parcel has shipped"}`)

	err := handler.Handle(context.Background(), messaging.Message{Body: body})
	if err == nil {
		t.Fatal("Handle() should have failed")
	}
	if messaging.IsPermanent(err) {
		t.Fatalf("Handle() classified an SMTP outage as permanent: %v", err)
	}
	if strings.Contains(logs.String(), "customer@example.com") {
		t.Fatalf("delivery log leaked the recipient: %s", logs.String())
	}
}

func TestHandleInvalidMessageIsPermanent(t *testing.T) {
	handler := New(
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		failingSender{err: mailer.ErrInvalidMessage},
	)
	body := envelopeBody(t, `{"recipient":"customer@example.com","channel":"email","template":"Your parcel has shipped"}`)

	err := handler.Handle(context.Background(), messaging.Message{Body: body})
	if err == nil || !messaging.IsPermanent(err) {
		t.Fatalf("Handle() error = %v, want a permanent error", err)
	}
}

func TestHandleRedactsRecipientFromDeliveryErrors(t *testing.T) {
	var logs bytes.Buffer
	handler := New(
		slog.New(slog.NewJSONHandler(&logs, nil)),
		failingSender{err: errors.New("550 customer@example.com rejected")},
	)
	body := envelopeBody(t, `{"recipient":"customer@example.com","channel":"email","template":"Your parcel has shipped"}`)

	err := handler.Handle(context.Background(), messaging.Message{Body: body})
	if err == nil {
		t.Fatal("Handle() should have failed")
	}
	if strings.Contains(err.Error(), "customer@example.com") {
		t.Fatalf("returned error leaked the recipient: %v", err)
	}
	if strings.Contains(logs.String(), "customer@example.com") {
		t.Fatalf("delivery log leaked the recipient: %s", logs.String())
	}
}

func TestHandleLogsOutcomeWithoutSensitivePayload(t *testing.T) {
	var logs bytes.Buffer
	sender := &mailer.Mock{}
	handler := New(slog.New(slog.NewJSONHandler(&logs, nil)), sender)
	body := envelopeBody(t, `{
		"recipient":"customer@example.com",
		"channel":"email",
		"template":"Your parcel has shipped",
		"variables":{"tracking_number":"SD-SECRET-VALUE"}
	}`)

	if err := handler.Handle(context.Background(), messaging.Message{Body: body}); err != nil {
		t.Fatalf("Handle() returned error: %v", err)
	}

	output := logs.String()
	for _, sensitive := range []string{"customer@example.com", "SD-SECRET-VALUE"} {
		if strings.Contains(output, sensitive) {
			t.Fatalf("success log leaked %q: %s", sensitive, output)
		}
	}
	for _, expected := range []string{"notification delivered", "event-1", "Your parcel has shipped"} {
		if !strings.Contains(output, expected) {
			t.Fatalf("success log missing %q: %s", expected, output)
		}
	}
}
