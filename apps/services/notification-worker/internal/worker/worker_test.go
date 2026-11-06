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
	"unicode/utf8"

	"github.com/Vineth-R/swiftdrop/apps/services/notification-worker/internal/idempotency"
	"github.com/Vineth-R/swiftdrop/apps/services/notification-worker/internal/identityclient"
	"github.com/Vineth-R/swiftdrop/packages/go-common/emails"
	"github.com/Vineth-R/swiftdrop/packages/go-common/mailer"
	"github.com/Vineth-R/swiftdrop/packages/go-common/messaging"
)

const (
	testEventID  = "11111111-1111-4111-8111-111111111111"
	testParcelID = "22222222-2222-4222-8222-222222222222"
)

type fakeStore struct {
	mu sync.Mutex

	processed map[string]bool
	delivered []idempotency.LogEntry
	failures  []idempotency.LogEntry

	lookupErr  error
	deliverErr error
	failureErr error

	lookups      int
	deliverCalls int
	failureCalls int
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

func (s *fakeStore) RecordDelivered(_ context.Context, entry idempotency.LogEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deliverCalls++
	entry.Status = idempotency.StatusSent
	s.delivered = append(s.delivered, entry)
	if s.deliverErr != nil {
		return s.deliverErr
	}
	s.processed[entry.EventID] = true
	return nil
}

func (s *fakeStore) RecordFailure(_ context.Context, entry idempotency.LogEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failureCalls++
	s.failures = append(s.failures, entry)
	return s.failureErr
}

func (s *fakeStore) setProcessed(eventID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.processed[eventID] = true
}

func (s *fakeStore) has(eventID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.processed[eventID]
}

func (s *fakeStore) recordCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.processed)
}

func (s *fakeStore) calls() (lookups, delivered, failures int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lookups, s.deliverCalls, s.failureCalls
}

func (s *fakeStore) deliveredEntries() []idempotency.LogEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]idempotency.LogEntry(nil), s.delivered...)
}

func (s *fakeStore) failureEntries() []idempotency.LogEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]idempotency.LogEntry(nil), s.failures...)
}

var testSite = emails.Site{BaseURL: "https://app.example.test", SupportEmail: "support@example.test"}

type fakeContacts struct {
	mu      sync.Mutex
	contact identityclient.Contact
	err     error
	calls   int
}

func newFakeContacts() *fakeContacts {
	return &fakeContacts{contact: identityclient.Contact{
		FullName: "Test Customer",
		Email:    "customer@example.test",
	}}
}

func (f *fakeContacts) Contact(_ context.Context, _ string) (identityclient.Contact, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.err != nil {
		return identityclient.Contact{}, f.err
	}
	return f.contact, nil
}

func (f *fakeContacts) setError(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.err = err
}

func (f *fakeContacts) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func testHandler(logger *slog.Logger, sender mailer.Mailer, store *fakeStore) *Handler {
	return New(logger, sender, store, newFakeContacts(), testSite)
}

func newFakeHandler() (*Handler, *mailer.Mock, *fakeStore) {
	sender := &mailer.Mock{}
	store := newFakeStore()
	return testHandler(slog.New(slog.NewTextHandler(io.Discard, nil)), sender, store), sender, store
}

func newDomainHandler() (*Handler, *mailer.Mock, *fakeStore, *fakeContacts) {
	sender := &mailer.Mock{}
	store := newFakeStore()
	contacts := newFakeContacts()
	handler := New(slog.New(slog.NewTextHandler(io.Discard, nil)), sender, store, contacts, testSite)
	return handler, sender, store, contacts
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
	return envelopeBodyWithData(t, eventID, addParcelID(t, data))
}

func envelopeBodyWithData(t *testing.T, eventID, data string) []byte {
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

func addParcelID(t *testing.T, data string) string {
	t.Helper()

	var object map[string]any
	if err := json.Unmarshal([]byte(data), &object); err != nil {
		return data
	}
	if _, ok := object["parcel_id"]; !ok {
		object["parcel_id"] = testParcelID
	}
	encoded, err := json.Marshal(object)
	if err != nil {
		t.Fatalf("add parcel_id: %v", err)
	}
	return string(encoded)
}

func TestHandleDeliversEmailAndPersistsSentLog(t *testing.T) {
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

	delivered := store.deliveredEntries()
	if len(delivered) != 1 {
		t.Fatalf("delivered entries = %d, want 1", len(delivered))
	}
	entry := delivered[0]
	if entry.EventID != testEventID || entry.ParcelID != testParcelID {
		t.Fatalf("entry identifiers = %s/%s", entry.EventID, entry.ParcelID)
	}
	if entry.Channel != "EMAIL" {
		t.Fatalf("channel = %q, want EMAIL", entry.Channel)
	}
	if entry.Status != idempotency.StatusSent {
		t.Fatalf("status = %q, want SENT", entry.Status)
	}
	if entry.Recipient != "customer@example.com" || entry.Subject != "Your parcel has shipped" {
		t.Fatalf("entry = %+v", entry)
	}
	if failures := store.failureEntries(); len(failures) != 0 {
		t.Fatalf("failure entries = %d, want 0", len(failures))
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
	validData := `{"parcel_id":"` + testParcelID + `","recipient":"customer@example.com","channel":"email","template":"Your parcel has shipped"}`

	envelopes := map[string]messaging.Envelope{
		"wrong event type": {
			ID: testEventID, Type: "parcel.assigned.v1", Version: 1,
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
			handler, sender, store := newFakeHandler()
			err := handler.Handle(context.Background(), messaging.Message{Body: envelopeBody(t, data)})
			if err == nil || !messaging.IsPermanent(err) {
				t.Fatalf("Handle() error = %v, want a permanent error", err)
			}
			if len(sender.Sent()) != 0 {
				t.Fatal("invalid data triggered delivery")
			}
			if lookups, delivered, failures := store.calls(); lookups != 0 || delivered != 0 || failures != 0 {
				t.Fatalf("invalid data touched the store: %d/%d/%d", lookups, delivered, failures)
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

func TestHandleRetryableDeliveryFailurePersistsPending(t *testing.T) {
	var logs bytes.Buffer
	store := newFakeStore()
	handler := testHandler(
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

	failures := store.failureEntries()
	if len(failures) != 1 {
		t.Fatalf("failure entries = %d, want 1", len(failures))
	}
	if failures[0].Status != idempotency.StatusPending {
		t.Fatalf("status = %q, want PENDING", failures[0].Status)
	}
	if failures[0].Channel != "EMAIL" {
		t.Fatalf("channel = %q, want EMAIL", failures[0].Channel)
	}
	if len(store.deliveredEntries()) != 0 {
		t.Fatal("retryable failure recorded a SENT log")
	}
}

func TestHandlePermanentDeliveryFailurePersistsFailed(t *testing.T) {
	store := newFakeStore()
	handler := testHandler(
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		failingSender{err: mailer.ErrInvalidMessage},
		store,
	)
	body := envelopeBody(t, `{"recipient":"customer@example.com","channel":"email","template":"Your parcel has shipped"}`)

	err := handler.Handle(context.Background(), messaging.Message{Body: body})
	if err == nil || !messaging.IsPermanent(err) {
		t.Fatalf("Handle() error = %v, want a permanent error", err)
	}
	failures := store.failureEntries()
	if len(failures) != 1 {
		t.Fatalf("failure entries = %d, want 1", len(failures))
	}
	if failures[0].Status != idempotency.StatusFailed {
		t.Fatalf("status = %q, want FAILED", failures[0].Status)
	}
	if store.has(testEventID) {
		t.Fatal("a permanent failure was recorded as processed")
	}
}

func TestHandlePersistsFailedForUnsupportedSMS(t *testing.T) {
	handler, sender, store := newFakeHandler()
	body := envelopeBody(t, `{"recipient":"customer@example.com","channel":"sms","template":"Your parcel has shipped"}`)

	err := handler.Handle(context.Background(), messaging.Message{Body: body})
	if err == nil || !messaging.IsPermanent(err) {
		t.Fatalf("Handle() error = %v, want a permanent error", err)
	}
	if len(sender.Sent()) != 0 {
		t.Fatal("SMS triggered an email delivery")
	}
	failures := store.failureEntries()
	if len(failures) != 1 {
		t.Fatalf("failure entries = %d, want 1", len(failures))
	}
	if failures[0].Channel != "SMS" || failures[0].Status != idempotency.StatusFailed {
		t.Fatalf("entry = %+v, want SMS/FAILED", failures[0])
	}
	if failures[0].ErrorMessage != "unsupported notification channel" {
		t.Fatalf("error = %q, want a generic unsupported-channel message", failures[0].ErrorMessage)
	}
	if store.has(testEventID) {
		t.Fatal("an unsupported channel was recorded as processed")
	}
}

func TestHandleRedactsRecipientFromDeliveryErrors(t *testing.T) {
	var logs bytes.Buffer
	store := newFakeStore()
	handler := testHandler(
		slog.New(slog.NewJSONHandler(&logs, nil)),
		failingSender{err: errors.New("550 customer@example.com rejected")},
		store,
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
	failures := store.failureEntries()
	if len(failures) != 1 || strings.Contains(failures[0].ErrorMessage, "customer@example.com") {
		t.Fatalf("persisted error leaked the recipient: %+v", failures)
	}
	if !strings.Contains(failures[0].ErrorMessage, "[REDACTED]") {
		t.Fatalf("persisted error did not mark the redaction: %q", failures[0].ErrorMessage)
	}
}

func TestHandleLogsOutcomeWithoutSensitivePayload(t *testing.T) {
	var logs bytes.Buffer
	sender := &mailer.Mock{}
	handler := testHandler(slog.New(slog.NewJSONHandler(&logs, nil)), sender, newFakeStore())
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
	for _, expected := range []string{"notification delivered", testEventID, `"channel":"EMAIL"`} {
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
			handler, sender, store := newFakeHandler()
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
			if lookups, delivered, failures := store.calls(); lookups != 0 || delivered != 0 || failures != 0 {
				t.Fatalf("invalid payload touched the store: %d/%d/%d", lookups, delivered, failures)
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
	if len(store.deliveredEntries()) != 0 || len(store.failureEntries()) != 0 {
		t.Fatal("a duplicate event wrote a notification log")
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
	if lookups, delivered, failures := store.calls(); lookups != 1 || delivered != 0 || failures != 0 {
		t.Fatalf("store calls = %d/%d/%d, want 1/0/0", lookups, delivered, failures)
	}
}

func TestHandleRetriesWhenDeliveredLogFails(t *testing.T) {
	handler, sender, store := newFakeHandler()
	store.deliverErr = errors.New("database unavailable")
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
	if lookups, delivered, _ := store.calls(); lookups != 1 || delivered != 1 {
		t.Fatalf("store calls = %d/%d, want 1/1", lookups, delivered)
	}
	if store.has(testEventID) {
		t.Fatal("a failed record was treated as processed")
	}
}

func TestHandleRetriesWhenFailureLogFails(t *testing.T) {
	store := newFakeStore()
	store.failureErr = errors.New("database unavailable")
	handler := testHandler(
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		failingSender{err: errors.New("smtp connection refused")},
		store,
	)
	body := envelopeBody(t, `{"recipient":"customer@example.com","channel":"email","template":"Your parcel has shipped"}`)

	err := handler.Handle(context.Background(), messaging.Message{Body: body})
	if err == nil {
		t.Fatal("Handle() should have failed when the failure log could not be written")
	}
	if messaging.IsPermanent(err) {
		t.Fatalf("failure-log error was classified as permanent: %v", err)
	}
	if store.has(testEventID) {
		t.Fatal("a failed log write recorded the event as processed")
	}
	if _, _, failures := store.calls(); failures != 1 {
		t.Fatalf("failure-log calls = %d, want 1", failures)
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
	if lookups, delivered, failures := store.calls(); lookups != 0 || delivered != 0 || failures != 0 {
		t.Fatalf("store calls = %d/%d/%d, want 0/0/0", lookups, delivered, failures)
	}
}

func TestHandleRejectsInvalidParcelID(t *testing.T) {
	tests := map[string]string{
		"missing": `{"recipient":"customer@example.com","channel":"email","template":"Your parcel has shipped"}`,
		"invalid": `{"parcel_id":"not-a-uuid","recipient":"customer@example.com","channel":"email","template":"Your parcel has shipped"}`,
	}

	for name, data := range tests {
		t.Run(name, func(t *testing.T) {
			handler, sender, store := newFakeHandler()
			body := envelopeBodyWithData(t, testEventID, data)

			err := handler.Handle(context.Background(), messaging.Message{Body: body})
			if err == nil || !messaging.IsPermanent(err) {
				t.Fatalf("Handle() error = %v, want a permanent error", err)
			}
			if len(sender.Sent()) != 0 {
				t.Fatal("an invalid parcel_id triggered delivery")
			}
			if lookups, delivered, failures := store.calls(); lookups != 0 || delivered != 0 || failures != 0 {
				t.Fatalf("store calls = %d/%d/%d, want 0/0/0", lookups, delivered, failures)
			}
		})
	}
}

func TestHandleRejectsOverlongRecipient(t *testing.T) {
	handler, sender, store := newFakeHandler()
	recipient := strings.Repeat("a", 250) + "@example.com"
	body := envelopeBody(t, `{"recipient":"`+recipient+`","channel":"email","template":"Your parcel has shipped"}`)

	err := handler.Handle(context.Background(), messaging.Message{Body: body})
	if err == nil || !messaging.IsPermanent(err) {
		t.Fatalf("Handle() error = %v, want a permanent error", err)
	}
	if len(sender.Sent()) != 0 {
		t.Fatal("an overlong recipient triggered delivery")
	}
	if lookups, delivered, failures := store.calls(); lookups != 0 || delivered != 0 || failures != 0 {
		t.Fatalf("store calls = %d/%d/%d, want 0/0/0", lookups, delivered, failures)
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

func TestHandleDoesNotLogStoreErrors(t *testing.T) {
	const (
		secret = "customer@example.com"
		marker = "SD-SECRET"
	)
	driverText := "failing row contains (" + secret + ", " + marker + ")"

	t.Run("lookup", func(t *testing.T) {
		var logs bytes.Buffer
		store := newFakeStore()
		store.lookupErr = errors.New(driverText)
		handler := testHandler(slog.New(slog.NewJSONHandler(&logs, nil)), &mailer.Mock{}, store)
		body := envelopeBody(t, `{"recipient":"customer@example.com","channel":"email","template":"Your parcel has shipped"}`)

		err := handler.Handle(context.Background(), messaging.Message{Body: body})
		assertStoreFailureIsSafe(t, err, logs.String(), secret, marker)
	})

	t.Run("delivered", func(t *testing.T) {
		var logs bytes.Buffer
		store := newFakeStore()
		store.deliverErr = errors.New(driverText)
		handler := testHandler(slog.New(slog.NewJSONHandler(&logs, nil)), &mailer.Mock{}, store)
		body := envelopeBody(t, `{"recipient":"customer@example.com","channel":"email","template":"Your parcel has shipped"}`)

		err := handler.Handle(context.Background(), messaging.Message{Body: body})
		assertStoreFailureIsSafe(t, err, logs.String(), secret, marker)
	})

	t.Run("failure record", func(t *testing.T) {
		var logs bytes.Buffer
		store := newFakeStore()
		store.failureErr = errors.New(driverText)
		handler := testHandler(
			slog.New(slog.NewJSONHandler(&logs, nil)),
			failingSender{err: errors.New("smtp connection refused")},
			store,
		)
		body := envelopeBody(t, `{"recipient":"customer@example.com","channel":"email","template":"Your parcel has shipped"}`)

		err := handler.Handle(context.Background(), messaging.Message{Body: body})
		assertStoreFailureIsSafe(t, err, logs.String(), secret, marker)
	})
}

func assertStoreFailureIsSafe(t *testing.T, err error, logs, secret, marker string) {
	t.Helper()

	if err == nil {
		t.Fatal("Handle() should have failed")
	}
	if messaging.IsPermanent(err) {
		t.Fatalf("store failure was classified as permanent: %v", err)
	}
	for _, value := range []string{secret, marker} {
		if strings.Contains(err.Error(), value) {
			t.Fatalf("returned error leaked %q: %v", value, err)
		}
		if strings.Contains(logs, value) {
			t.Fatalf("logs leaked %q: %s", value, logs)
		}
	}
}

func TestSanitizeErrorMessage(t *testing.T) {
	t.Run("redacts recipient and control characters", func(t *testing.T) {
		got := sanitizeErrorMessage("550 customer@example.com\r\nrejected\x00 now", "customer@example.com")
		if strings.Contains(got, "customer@example.com") {
			t.Fatalf("recipient was not redacted: %q", got)
		}
		if strings.ContainsAny(got, "\r\n\x00") {
			t.Fatalf("control characters were not removed: %q", got)
		}
		if !strings.Contains(got, "[REDACTED]") {
			t.Fatalf("redaction marker missing: %q", got)
		}
	})

	t.Run("truncates to the database limit without splitting UTF-8", func(t *testing.T) {
		value := strings.Repeat("é", 200)
		got := sanitizeErrorMessage(value, "")
		if len(got) > 255 {
			t.Fatalf("length = %d, want at most 255", len(got))
		}
		if !utf8.ValidString(got) {
			t.Fatalf("truncated value is not valid UTF-8: %q", got)
		}
	})

	t.Run("collapses whitespace", func(t *testing.T) {
		if got := sanitizeErrorMessage("  a\t\tb  ", ""); got != "a b" {
			t.Fatalf("got %q, want %q", got, "a b")
		}
	})
}
