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
	"sync"
	"testing"
	"time"

	"github.com/Vineth-R/swiftdrop/packages/go-common/mailer"
	"github.com/Vineth-R/swiftdrop/packages/go-common/messaging"
)

const testEventID = "11111111-1111-4111-8111-111111111111"

type fakeStore struct {
	mu        sync.Mutex
	processed map[string]bool
	lookupErr error
	markErr   error
	lookups   int
	marks     int
}

func newFakeStore() *fakeStore {
	return &fakeStore{processed: map[string]bool{}}
}

func (s *fakeStore) IsProcessed(_ context.Context, eventID string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lookups++
	if s.lookupErr != nil {
		return false, s.lookupErr
	}
	return s.processed[eventID], nil
}

func (s *fakeStore) MarkProcessed(_ context.Context, eventID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.marks++
	if s.markErr != nil {
		return s.markErr
	}
	s.processed[eventID] = true
	return nil
}

func (s *fakeStore) has(eventID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.processed[eventID]
}

func (s *fakeStore) setProcessed(eventID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.processed[eventID] = true
}

func (s *fakeStore) recordCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.processed)
}

func (s *fakeStore) calls() (lookups, marks int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lookups, s.marks
}

func newFakeHandler() (*Handler, *mailer.Mock, *fakeStore) {
	sender := &mailer.Mock{}
	store := newFakeStore()
	return New(slog.New(slog.NewTextHandler(io.Discard, nil)), sender, store), sender, store
}

func newHandler() (*Handler, *mailer.Mock) {
	handler, sender, _ := newFakeHandler()
	return handler, sender
}

func envelopeBody(t *testing.T, data string) []byte {
	t.Helper()
	return envelopeBodyWithID(t, testEventID, data)
}

func envelopeBodyWithID(t *testing.T, eventID, data string) []byte {
	t.Helper()

	body, err := json.Marshal(messaging.Envelope{
		ID:         eventID,
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
	handler, sender, store := newFakeHandler()
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
	if !store.has(testEventID) {
		t.Fatal("successful delivery was not recorded as processed")
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
			ID: testEventID, Type: "parcel.created.v1", Version: 1,
			OccurredAt: time.Now().UTC(), Producer: "parcel", Data: json.RawMessage(validData),
		},
		"wrong event version": {
			ID: testEventID, Type: EventType, Version: 2,
			OccurredAt: time.Now().UTC(), Producer: "parcel", Data: json.RawMessage(validData),
		},
		"invalid envelope": {
			ID: testEventID, Type: EventType, Version: 1,
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
		body := []byte(`{"event_id":"11111111-1111-4111-8111-111111111111","event_type":"notification.requested.v1","event_version":1,"occurred_at":"2026-11-02T09:30:00Z","producer":"parcel","data":{"recipient":}}`)
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
		"null variables":             `{"recipient":"customer@example.com","channel":"email","template":"hello","variables":null}`,
		"array variables":            `{"recipient":"customer@example.com","channel":"email","template":"hello","variables":[]}`,
		"string variables":           `{"recipient":"customer@example.com","channel":"email","template":"hello","variables":"not-an-object"}`,
		"number variables":           `{"recipient":"customer@example.com","channel":"email","template":"hello","variables":7}`,
		"non-string variable value":  `{"recipient":"customer@example.com","channel":"email","template":"hello","variables":{"key":7}}`,
		"variable key whitespace":    `{"recipient":"customer@example.com","channel":"email","template":"hello","variables":{" key ":"value"}}`,
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
	store := newFakeStore()
	handler := New(
		slog.New(slog.NewJSONHandler(&logs, nil)),
		failingSender{err: errors.New("smtp connection refused")},
		store,
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
	if store.has(testEventID) {
		t.Fatal("a failed delivery was recorded as processed")
	}
}

func TestHandleInvalidMessageIsPermanent(t *testing.T) {
	handler := New(
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		failingSender{err: mailer.ErrInvalidMessage},
		newFakeStore(),
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
		newFakeStore(),
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
	handler := New(slog.New(slog.NewJSONHandler(&logs, nil)), sender, newFakeStore())
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
	for _, expected := range []string{"notification delivered", testEventID, `"channel":"email"`} {
		if !strings.Contains(output, expected) {
			t.Fatalf("success log missing %q: %s", expected, output)
		}
	}
}

func TestHandleDoesNotEchoUntrustedPayloadValues(t *testing.T) {
	tests := []struct {
		name   string
		data   string
		secret string
	}{
		{
			name:   "unknown channel",
			data:   `{"recipient":"customer@example.com","channel":"secret-channel","template":"hello"}`,
			secret: "secret-channel",
		},
		{
			name:   "overlong variable value",
			data:   `{"recipient":"customer@example.com","channel":"email","template":"hello","variables":{"secret-var-key":"` + strings.Repeat("v", maxVariableVal+1) + `"}}`,
			secret: "secret-var-key",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			handler, sender := newHandler()
			err := handler.Handle(context.Background(), messaging.Message{Body: envelopeBody(t, test.data)})
			if err == nil || !messaging.IsPermanent(err) {
				t.Fatalf("Handle() error = %v, want a permanent error", err)
			}
			if strings.Contains(err.Error(), test.secret) {
				t.Fatalf("validation error echoed untrusted payload value %q: %v", test.secret, err)
			}
			if len(sender.Sent()) != 0 {
				t.Fatal("invalid payload triggered delivery")
			}
		})
	}
}

func TestHandleSkipsAlreadyProcessedEvent(t *testing.T) {
	handler, sender, store := newFakeHandler()
	store.setProcessed(testEventID)
	body := envelopeBody(t, `{"recipient":"customer@example.com","channel":"email","template":"Your parcel has shipped"}`)

	if err := handler.Handle(context.Background(), messaging.Message{Body: body}); err != nil {
		t.Fatalf("Handle() returned error: %v", err)
	}
	if len(sender.Sent()) != 0 {
		t.Fatal("a duplicate event triggered a second delivery")
	}
	_, marks := store.calls()
	if marks != 0 {
		t.Fatalf("duplicate event marked processed %d times, want 0", marks)
	}
}

func TestHandleRetriesWhenLookupFails(t *testing.T) {
	handler, sender, store := newFakeHandler()
	store.lookupErr = errors.New("database unavailable")
	body := envelopeBody(t, `{"recipient":"customer@example.com","channel":"email","template":"Your parcel has shipped"}`)

	err := handler.Handle(context.Background(), messaging.Message{Body: body})
	if err == nil {
		t.Fatal("Handle() should have failed when the lookup failed")
	}
	if messaging.IsPermanent(err) {
		t.Fatalf("lookup failure was classified as permanent: %v", err)
	}
	if len(sender.Sent()) != 0 {
		t.Fatal("a lookup failure triggered delivery")
	}
	_, marks := store.calls()
	if marks != 0 {
		t.Fatalf("lookup failure marked processed %d times, want 0", marks)
	}
}

func TestHandleDoesNotMarkProcessedWhenDeliveryFails(t *testing.T) {
	store := newFakeStore()
	handler := New(
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		failingSender{err: errors.New("smtp connection refused")},
		store,
	)
	body := envelopeBody(t, `{"recipient":"customer@example.com","channel":"email","template":"Your parcel has shipped"}`)

	if err := handler.Handle(context.Background(), messaging.Message{Body: body}); err == nil {
		t.Fatal("Handle() should have failed")
	}
	_, marks := store.calls()
	if marks != 0 {
		t.Fatalf("failed delivery marked processed %d times, want 0", marks)
	}
}

func TestHandleRetriesWhenMarkFails(t *testing.T) {
	handler, sender, store := newFakeHandler()
	store.markErr = errors.New("database unavailable")
	body := envelopeBody(t, `{"recipient":"customer@example.com","channel":"email","template":"Your parcel has shipped"}`)

	err := handler.Handle(context.Background(), messaging.Message{Body: body})
	if err == nil {
		t.Fatal("Handle() should have failed when recording failed")
	}
	if messaging.IsPermanent(err) {
		t.Fatalf("record failure was classified as permanent: %v", err)
	}
	if len(sender.Sent()) != 1 {
		t.Fatalf("sent messages = %d, want 1", len(sender.Sent()))
	}
	_, marks := store.calls()
	if marks != 1 {
		t.Fatalf("mark calls = %d, want 1", marks)
	}
	if store.has(testEventID) {
		t.Fatal("a failed record was treated as processed")
	}
}

func TestHandleRejectsNonUUIDEventID(t *testing.T) {
	handler, sender, store := newFakeHandler()
	body := envelopeBodyWithID(t, "event-1",
		`{"recipient":"customer@example.com","channel":"email","template":"Your parcel has shipped"}`)

	err := handler.Handle(context.Background(), messaging.Message{Body: body})
	if err == nil || !messaging.IsPermanent(err) {
		t.Fatalf("Handle() error = %v, want a permanent error", err)
	}
	if len(sender.Sent()) != 0 {
		t.Fatal("a non-UUID event ID triggered delivery")
	}
	lookups, marks := store.calls()
	if lookups != 0 || marks != 0 {
		t.Fatalf("store calls = %d lookups/%d marks, want 0/0", lookups, marks)
	}
}

func TestHandleConcurrentDuplicatesRecordOnce(t *testing.T) {
	handler, sender, store := newFakeHandler()
	body := envelopeBody(t, `{"recipient":"customer@example.com","channel":"email","template":"Your parcel has shipped"}`)

	const workers = 8
	var wait sync.WaitGroup
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			errs <- handler.Handle(context.Background(), messaging.Message{Body: body})
		}()
	}
	wait.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent Handle() returned error: %v", err)
		}
	}
	if got := store.recordCount(); got != 1 {
		t.Fatalf("processed records = %d, want 1", got)
	}
	if len(sender.Sent()) < 1 {
		t.Fatal("no email was delivered")
	}
}
