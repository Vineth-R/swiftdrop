package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/Vineth-R/swiftdrop/apps/services/notification-worker/internal/idempotency"
	"github.com/Vineth-R/swiftdrop/apps/services/notification-worker/internal/identityclient"
	"github.com/Vineth-R/swiftdrop/packages/go-common/mailer"
	"github.com/Vineth-R/swiftdrop/packages/go-common/messaging"
)

const (
	testCustomerID = "3f1c7a52-0000-4000-8000-000000000001"
	testMerchantID = "4f1c7a52-0000-4000-8000-000000000002"
	testTracking   = "SDK7M2Q9XP4"
)

func parcelEnvelope(t *testing.T, eventID, eventType string, data any) []byte {
	t.Helper()
	return parcelEnvelopeWithProducer(t, eventID, eventType, "parcel", data)
}

func parcelEnvelopeWithProducer(t *testing.T, eventID, eventType, producer string, data any) []byte {
	t.Helper()
	encoded, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("marshal data: %v", err)
	}
	body, err := json.Marshal(messaging.Envelope{
		ID:            eventID,
		Type:          eventType,
		Version:       1,
		OccurredAt:    time.Now().UTC(),
		Producer:      producer,
		CorrelationID: "corr-" + eventID,
		Data:          encoded,
	})
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}
	return body
}

func createdData() map[string]any {
	return map[string]any{
		"parcel_id":        testParcelID,
		"tracking_number":  testTracking,
		"status":           "CREATED",
		"customer_user_id": testCustomerID,
	}
}

func statusData(previous, status, reason string) map[string]any {
	data := map[string]any{
		"parcel_id":        testParcelID,
		"tracking_number":  testTracking,
		"previous_status":  previous,
		"status":           status,
		"customer_user_id": testCustomerID,
	}
	if reason != "" {
		data["failure_reason"] = reason
	}
	return data
}

func merchantData(data map[string]any) map[string]any {
	delete(data, "customer_user_id")
	data["merchant_id"] = testMerchantID
	return data
}

func TestHandleParcelCreatedSendsEmailAndPersists(t *testing.T) {
	handler, sender, store, contacts := newDomainHandler()
	body := parcelEnvelope(t, testEventID, ParcelCreatedEventType, createdData())

	if err := handler.Handle(context.Background(), messaging.Message{Body: body}); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	sent := sender.Sent()
	if len(sent) != 1 {
		t.Fatalf("sent = %d emails, want 1", len(sent))
	}
	if sent[0].To != "customer@example.test" {
		t.Errorf("To = %q", sent[0].To)
	}
	if !strings.Contains(sent[0].Subject, testTracking) {
		t.Errorf("subject = %q, want the tracking number", sent[0].Subject)
	}
	for _, want := range []string{"Test Customer", "Current status: Created", "Tracking number: " + testTracking} {
		if !strings.Contains(sent[0].Text, want) {
			t.Errorf("text does not contain %q:\n%s", want, sent[0].Text)
		}
	}
	if !strings.Contains(sent[0].HTML, testTracking) {
		t.Error("HTML does not contain the tracking number")
	}

	if contacts.callCount() != 1 {
		t.Errorf("contact lookups = %d, want 1", contacts.callCount())
	}
	if !store.has(testEventID) {
		t.Error("event was not recorded as processed")
	}
	delivered := store.deliveredEntries()
	if len(delivered) != 1 {
		t.Fatalf("delivered entries = %d, want 1", len(delivered))
	}
	entry := delivered[0]
	if entry.EventID != testEventID || entry.ParcelID != testParcelID || entry.Channel != "EMAIL" ||
		entry.Recipient != "customer@example.test" || entry.Subject != sent[0].Subject {
		t.Errorf("delivered entry = %+v", entry)
	}
}

func TestHandleParcelStatusChangedSendsStatusSpecificEmail(t *testing.T) {
	tests := []struct {
		previous, status, reason, wantSubject, wantStatus string
	}{
		{previous: "CREATED", status: "PICKED_UP", wantSubject: "picked up", wantStatus: "Picked up"},
		{previous: "PICKED_UP", status: "IN_TRANSIT", wantSubject: "in transit", wantStatus: "In transit"},
		{previous: "IN_TRANSIT", status: "OUT_FOR_DELIVERY", wantSubject: "out for delivery", wantStatus: "Out for delivery"},
		{previous: "OUT_FOR_DELIVERY", status: "DELIVERED", wantSubject: "delivered", wantStatus: "Delivered"},
		{previous: "IN_TRANSIT", status: "FAILED", reason: "Recipient refused delivery", wantSubject: "could not deliver", wantStatus: "Failed"},
	}

	for _, test := range tests {
		t.Run(test.status, func(t *testing.T) {
			handler, sender, store, contacts := newDomainHandler()
			body := parcelEnvelope(t, testEventID, ParcelStatusChangedEventType, statusData(test.previous, test.status, test.reason))

			if err := handler.Handle(context.Background(), messaging.Message{Body: body}); err != nil {
				t.Fatalf("Handle() error = %v", err)
			}
			sent := sender.Sent()
			if len(sent) != 1 {
				t.Fatalf("sent = %d emails, want 1", len(sent))
			}
			if !strings.Contains(strings.ToLower(sent[0].Subject), test.wantSubject) {
				t.Errorf("subject = %q, want %q", sent[0].Subject, test.wantSubject)
			}
			if !strings.Contains(sent[0].Text, "Tracking number: "+testTracking) {
				t.Errorf("text does not contain the tracking number:\n%s", sent[0].Text)
			}
			if !strings.Contains(sent[0].Text, "Current status: "+test.wantStatus) {
				t.Errorf("text does not contain the current status %q:\n%s", test.wantStatus, sent[0].Text)
			}
			if test.reason != "" && !strings.Contains(sent[0].Text, test.reason) {
				t.Errorf("text does not contain the failure reason:\n%s", sent[0].Text)
			}
			if contacts.callCount() != 1 {
				t.Errorf("contact lookups = %d, want 1", contacts.callCount())
			}
			if len(store.deliveredEntries()) != 1 {
				t.Errorf("delivered entries = %d, want 1", len(store.deliveredEntries()))
			}
		})
	}
}

func TestHandleParcelEventsRejectForeignProducer(t *testing.T) {
	tests := []struct {
		name      string
		eventType string
		producer  string
		data      any
	}{
		{name: "created foreign producer", eventType: ParcelCreatedEventType, producer: "identity", data: createdData()},
		{name: "created empty producer", eventType: ParcelCreatedEventType, producer: "", data: createdData()},
		{name: "status foreign producer", eventType: ParcelStatusChangedEventType, producer: "merchant", data: statusData("CREATED", "PICKED_UP", "")},
		{name: "status empty producer", eventType: ParcelStatusChangedEventType, producer: "", data: statusData("CREATED", "PICKED_UP", "")},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			handler, sender, store, contacts := newDomainHandler()
			body := parcelEnvelopeWithProducer(t, testEventID, test.eventType, test.producer, test.data)

			err := handler.Handle(context.Background(), messaging.Message{Body: body})
			if err == nil || !messaging.IsPermanent(err) {
				t.Fatalf("Handle() error = %v, want a permanent error", err)
			}
			if len(sender.Sent()) != 0 {
				t.Error("foreign-producer event sent an email")
			}
			if contacts.callCount() != 0 {
				t.Error("foreign-producer event looked up a contact")
			}
			if lookups, delivered, failures := store.calls(); lookups != 0 || delivered != 0 || failures != 0 {
				t.Errorf("foreign-producer event touched the store: %d/%d/%d", lookups, delivered, failures)
			}
			if store.has(testEventID) {
				t.Error("foreign-producer event marked the event processed")
			}
		})
	}
}

func TestHandleParcelMerchantOwnedIsUnsupported(t *testing.T) {
	tests := []struct {
		name      string
		eventType string
		data      map[string]any
	}{
		{name: "created", eventType: ParcelCreatedEventType, data: merchantData(createdData())},
		{name: "status changed", eventType: ParcelStatusChangedEventType, data: merchantData(statusData("CREATED", "PICKED_UP", ""))},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			handler, sender, store, contacts := newDomainHandler()
			body := parcelEnvelope(t, testEventID, test.eventType, test.data)

			err := handler.Handle(context.Background(), messaging.Message{Body: body})
			if err == nil || !messaging.IsPermanent(err) {
				t.Fatalf("Handle() error = %v, want a permanent error", err)
			}
			if len(sender.Sent()) != 0 {
				t.Error("merchant-owned event sent an email")
			}
			if contacts.callCount() != 0 {
				t.Error("merchant-owned event looked up a contact")
			}
			if lookups, delivered, failures := store.calls(); lookups != 0 || delivered != 0 || failures != 0 {
				t.Errorf("merchant-owned event touched the store: %d/%d/%d", lookups, delivered, failures)
			}
		})
	}
}

func TestHandleParcelEventsRejectInvalidData(t *testing.T) {
	missingParcel := createdData()
	delete(missingParcel, "parcel_id")

	notCreated := createdData()
	notCreated["status"] = "PICKED_UP"

	noOwner := createdData()
	delete(noOwner, "customer_user_id")

	bothOwners := createdData()
	bothOwners["merchant_id"] = testMerchantID

	badCustomer := createdData()
	badCustomer["customer_user_id"] = "not-a-uuid"

	shortTracking := createdData()
	shortTracking["tracking_number"] = "short"

	badPrevious := statusData("NOT_A_STATUS", "PICKED_UP", "")
	badStatus := statusData("CREATED", "NOT_A_STATUS", "")
	failedWithoutReason := statusData("IN_TRANSIT", "FAILED", "")
	failedShortReason := statusData("IN_TRANSIT", "FAILED", "no")
	statusNoOwner := statusData("CREATED", "PICKED_UP", "")
	delete(statusNoOwner, "customer_user_id")
	statusBothOwners := statusData("CREATED", "PICKED_UP", "")
	statusBothOwners["merchant_id"] = testMerchantID
	statusBadCustomer := statusData("CREATED", "PICKED_UP", "")
	statusBadCustomer["customer_user_id"] = "not-a-uuid"
	statusBadMerchant := merchantData(statusData("CREATED", "PICKED_UP", ""))
	statusBadMerchant["merchant_id"] = "not-a-uuid"

	longTracking := createdData()
	longTracking["tracking_number"] = strings.Repeat("A", 21)

	failedLongReason := statusData("IN_TRANSIT", "FAILED", strings.Repeat("r", 256))

	missingStatus := statusData("CREATED", "", "")
	delete(missingStatus, "status")

	emptyStatus := statusData("CREATED", "", "")

	missingPrevious := statusData("", "PICKED_UP", "")
	delete(missingPrevious, "previous_status")

	emptyPrevious := statusData("", "PICKED_UP", "")

	statusBadActor := statusData("CREATED", "PICKED_UP", "")
	statusBadActor["actor_user_id"] = "not-a-uuid"

	statusLongNote := statusData("CREATED", "PICKED_UP", "")
	statusLongNote["note"] = strings.Repeat("n", 501)

	tests := []struct {
		name      string
		eventType string
		data      any
	}{
		{name: "created missing parcel", eventType: ParcelCreatedEventType, data: missingParcel},
		{name: "created wrong status", eventType: ParcelCreatedEventType, data: notCreated},
		{name: "created no owner", eventType: ParcelCreatedEventType, data: noOwner},
		{name: "created both owners", eventType: ParcelCreatedEventType, data: bothOwners},
		{name: "created invalid customer", eventType: ParcelCreatedEventType, data: badCustomer},
		{name: "created short tracking", eventType: ParcelCreatedEventType, data: shortTracking},
		{name: "created long tracking", eventType: ParcelCreatedEventType, data: longTracking},
		{name: "status invalid previous", eventType: ParcelStatusChangedEventType, data: badPrevious},
		{name: "status invalid status", eventType: ParcelStatusChangedEventType, data: badStatus},
		{name: "status missing status", eventType: ParcelStatusChangedEventType, data: missingStatus},
		{name: "status empty status", eventType: ParcelStatusChangedEventType, data: emptyStatus},
		{name: "status missing previous", eventType: ParcelStatusChangedEventType, data: missingPrevious},
		{name: "status empty previous", eventType: ParcelStatusChangedEventType, data: emptyPrevious},
		{name: "failed without reason", eventType: ParcelStatusChangedEventType, data: failedWithoutReason},
		{name: "failed short reason", eventType: ParcelStatusChangedEventType, data: failedShortReason},
		{name: "failed long reason", eventType: ParcelStatusChangedEventType, data: failedLongReason},
		{name: "status no owner", eventType: ParcelStatusChangedEventType, data: statusNoOwner},
		{name: "status both owners", eventType: ParcelStatusChangedEventType, data: statusBothOwners},
		{name: "status invalid customer", eventType: ParcelStatusChangedEventType, data: statusBadCustomer},
		{name: "status invalid merchant", eventType: ParcelStatusChangedEventType, data: statusBadMerchant},
		{name: "status invalid actor", eventType: ParcelStatusChangedEventType, data: statusBadActor},
		{name: "status long note", eventType: ParcelStatusChangedEventType, data: statusLongNote},
		{name: "unsupported event type", eventType: "parcel.assigned.v1", data: createdData()},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			handler, sender, store, contacts := newDomainHandler()
			body := parcelEnvelope(t, testEventID, test.eventType, test.data)

			err := handler.Handle(context.Background(), messaging.Message{Body: body})
			if err == nil || !messaging.IsPermanent(err) {
				t.Fatalf("Handle() error = %v, want a permanent error", err)
			}
			if len(sender.Sent()) != 0 {
				t.Error("invalid event sent an email")
			}
			if contacts.callCount() != 0 {
				t.Error("invalid event looked up a contact")
			}
			if lookups, delivered, failures := store.calls(); lookups != 0 || delivered != 0 || failures != 0 {
				t.Errorf("invalid event touched the store: %d/%d/%d", lookups, delivered, failures)
			}
		})
	}
}

func TestHandleParcelContactLookupFailures(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		permanent bool
	}{
		{name: "not found", err: identityclient.ErrNotFound, permanent: true},
		{name: "unauthorized", err: identityclient.ErrUnauthorized, permanent: true},
		{name: "invalid response", err: identityclient.ErrInvalidResponse, permanent: true},
		{name: "unavailable", err: identityclient.ErrUnavailable, permanent: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			handler, sender, store, contacts := newDomainHandler()
			contacts.setError(test.err)
			body := parcelEnvelope(t, testEventID, ParcelCreatedEventType, createdData())

			err := handler.Handle(context.Background(), messaging.Message{Body: body})
			if err == nil {
				t.Fatal("Handle() error = nil")
			}
			if got := messaging.IsPermanent(err); got != test.permanent {
				t.Fatalf("IsPermanent(error) = %t, want %t (error: %v)", got, test.permanent, err)
			}
			if len(sender.Sent()) != 0 {
				t.Error("contact failure sent an email")
			}
			if lookups, delivered, failures := store.calls(); lookups != 1 || delivered != 0 || failures != 0 {
				t.Errorf("store calls = %d/%d/%d, want 1/0/0", lookups, delivered, failures)
			}
			if store.has(testEventID) {
				t.Error("contact failure marked the event processed")
			}
		})
	}
}

func TestHandleParcelDuplicateSkipsContactAndEmail(t *testing.T) {
	handler, sender, store, contacts := newDomainHandler()
	store.setProcessed(testEventID)
	body := parcelEnvelope(t, testEventID, ParcelCreatedEventType, createdData())

	if err := handler.Handle(context.Background(), messaging.Message{Body: body}); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if len(sender.Sent()) != 0 {
		t.Error("duplicate event sent an email")
	}
	if contacts.callCount() != 0 {
		t.Error("duplicate event looked up a contact")
	}
	if lookups, delivered, failures := store.calls(); lookups != 1 || delivered != 0 || failures != 0 {
		t.Errorf("store calls = %d/%d/%d, want 1/0/0", lookups, delivered, failures)
	}
}

func TestHandleParcelSMTPFailure(t *testing.T) {
	handler, sender, store, _ := newDomainHandler()
	sender.FailWith(errors.New("smtp connection refused"))
	body := parcelEnvelope(t, testEventID, ParcelCreatedEventType, createdData())

	err := handler.Handle(context.Background(), messaging.Message{Body: body})
	if err == nil || messaging.IsPermanent(err) {
		t.Fatalf("Handle() error = %v, want a retryable error", err)
	}
	failures := store.failureEntries()
	if len(failures) != 1 || failures[0].Status != idempotency.StatusPending {
		t.Fatalf("failure entries = %+v, want one PENDING", failures)
	}
	if store.has(testEventID) {
		t.Error("SMTP failure marked the event processed")
	}
	if len(store.deliveredEntries()) != 0 {
		t.Error("SMTP failure recorded a delivered notification")
	}
}

func TestHandleParcelInvalidMessage(t *testing.T) {
	handler, sender, store, _ := newDomainHandler()
	sender.FailWith(mailer.ErrInvalidMessage)
	body := parcelEnvelope(t, testEventID, ParcelCreatedEventType, createdData())

	err := handler.Handle(context.Background(), messaging.Message{Body: body})
	if err == nil || !messaging.IsPermanent(err) {
		t.Fatalf("Handle() error = %v, want a permanent error", err)
	}
	failures := store.failureEntries()
	if len(failures) != 1 || failures[0].Status != idempotency.StatusFailed {
		t.Fatalf("failure entries = %+v, want one FAILED", failures)
	}
	if store.has(testEventID) {
		t.Error("permanent SMTP failure marked the event processed")
	}
}

func TestHandleParcelDoesNotLogContactData(t *testing.T) {
	const (
		name  = "Private Person"
		email = "private@example.test"
	)
	var logs bytes.Buffer
	store := newFakeStore()
	contacts := newFakeContacts()
	contacts.contact = identityclient.Contact{FullName: name, Email: email}
	handler := New(
		slog.New(slog.NewJSONHandler(&logs, nil)),
		failingSender{err: errors.New("550 " + email + " rejected")},
		store,
		contacts,
		testSite,
	)
	body := parcelEnvelope(t, testEventID, ParcelCreatedEventType, createdData())

	err := handler.Handle(context.Background(), messaging.Message{Body: body})
	if err == nil {
		t.Fatal("Handle() error = nil")
	}
	for _, value := range []string{name, email} {
		if strings.Contains(err.Error(), value) || strings.Contains(logs.String(), value) {
			t.Fatalf("contact data %q leaked into the error or logs", value)
		}
	}
}

func TestHandleParcelStoreFailures(t *testing.T) {
	const (
		secret = "private@example.test"
		marker = "SD-DRIVER"
	)
	driverText := "failing row contains (" + secret + ", " + marker + ")"

	t.Run("lookup", func(t *testing.T) {
		var logs bytes.Buffer
		store := newFakeStore()
		store.lookupErr = errors.New(driverText)
		handler := New(slog.New(slog.NewJSONHandler(&logs, nil)), &mailer.Mock{}, store, newFakeContacts(), testSite)
		body := parcelEnvelope(t, testEventID, ParcelCreatedEventType, createdData())
		err := handler.Handle(context.Background(), messaging.Message{Body: body})
		assertStoreFailureIsSafe(t, err, logs.String(), secret, marker)
	})

	t.Run("delivered", func(t *testing.T) {
		var logs bytes.Buffer
		store := newFakeStore()
		store.deliverErr = errors.New(driverText)
		handler := New(slog.New(slog.NewJSONHandler(&logs, nil)), &mailer.Mock{}, store, newFakeContacts(), testSite)
		body := parcelEnvelope(t, testEventID, ParcelCreatedEventType, createdData())
		err := handler.Handle(context.Background(), messaging.Message{Body: body})
		assertStoreFailureIsSafe(t, err, logs.String(), secret, marker)
	})

	t.Run("failure record", func(t *testing.T) {
		var logs bytes.Buffer
		store := newFakeStore()
		store.failureErr = errors.New(driverText)
		handler := New(
			slog.New(slog.NewJSONHandler(&logs, nil)),
			failingSender{err: errors.New("smtp connection refused")},
			store,
			newFakeContacts(),
			testSite,
		)
		body := parcelEnvelope(t, testEventID, ParcelCreatedEventType, createdData())
		err := handler.Handle(context.Background(), messaging.Message{Body: body})
		assertStoreFailureIsSafe(t, err, logs.String(), secret, marker)
	})
}

func TestHandleParcelWithoutContactResolver(t *testing.T) {
	store := newFakeStore()
	handler := New(slog.New(slog.NewTextHandler(bytes.NewBuffer(nil), nil)), &mailer.Mock{}, store, nil, testSite)
	body := parcelEnvelope(t, testEventID, ParcelCreatedEventType, createdData())

	err := handler.Handle(context.Background(), messaging.Message{Body: body})
	if err == nil || messaging.IsPermanent(err) {
		t.Fatalf("Handle() error = %v, want a retryable configuration error", err)
	}
	if lookups, delivered, failures := store.calls(); lookups != 1 || delivered != 0 || failures != 0 {
		t.Errorf("store calls = %d/%d/%d, want 1/0/0", lookups, delivered, failures)
	}
}

func TestHandleParcelNonUUIDEventID(t *testing.T) {
	handler, sender, store, contacts := newDomainHandler()
	body := parcelEnvelope(t, "not-a-uuid", ParcelCreatedEventType, createdData())

	err := handler.Handle(context.Background(), messaging.Message{Body: body})
	if err == nil || !messaging.IsPermanent(err) {
		t.Fatalf("Handle() error = %v, want a permanent error", err)
	}
	if len(sender.Sent()) != 0 || contacts.callCount() != 0 {
		t.Error("invalid event ID triggered a lookup or delivery")
	}
	if lookups, delivered, failures := store.calls(); lookups != 0 || delivered != 0 || failures != 0 {
		t.Errorf("invalid event ID touched the store: %d/%d/%d", lookups, delivered, failures)
	}
}
