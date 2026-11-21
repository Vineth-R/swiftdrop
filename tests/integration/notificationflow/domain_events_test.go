//go:build notificationflow

package notificationflow

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Vineth-R/swiftdrop/packages/go-common/messaging"
)

func domainEnvelope(t *testing.T, eventID, eventType string, data map[string]any) []byte {
	t.Helper()
	return domainEnvelopeWithProducer(t, eventID, eventType, "parcel", data)
}

func domainEnvelopeWithProducer(t *testing.T, eventID, eventType, producer string, data map[string]any) []byte {
	t.Helper()
	encoded, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("encode %s data: %v", eventType, err)
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
		t.Fatalf("encode %s envelope: %v", eventType, err)
	}
	return body
}

func publishDomain(t *testing.T, h *harness, eventID string, body []byte, routingKey string) {
	t.Helper()
	if err := h.publishTo(eventID, string(body), routingKey); err != nil {
		t.Fatalf("publish domain event: %v", err)
	}
}

func domainCreatedData(customerID, parcelID, tracking string) map[string]any {
	return map[string]any{
		"parcel_id":        parcelID,
		"tracking_number":  tracking,
		"status":           "CREATED",
		"customer_user_id": customerID,
	}
}

func domainStatusData(customerID, parcelID, tracking, previous, status, reason string) map[string]any {
	data := map[string]any{
		"parcel_id":        parcelID,
		"tracking_number":  tracking,
		"previous_status":  previous,
		"status":           status,
		"customer_user_id": customerID,
	}
	if reason != "" {
		data["failure_reason"] = reason
	}
	return data
}

func assertNotificationState(t *testing.T, h *harness, eventID, recipient, tracking string) {
	t.Helper()
	h.waitForStatus(t, eventID, recipient, "SENT", 30*time.Second)
	h.waitForProcessed(t, eventID, 1, 30*time.Second)
	if got := h.count(t, "SELECT count(*) FROM notification.notification_logs WHERE event_id = "+sqlLiteral(eventID)); got != 1 {
		t.Errorf("notification_logs rows for %s = %d, want 1", eventID, got)
	}
	subject := h.scalar(t, "SELECT subject FROM notification.notification_logs WHERE event_id = "+sqlLiteral(eventID))
	if !strings.Contains(subject, tracking) {
		t.Errorf("notification log subject = %q, want the tracking number", subject)
	}
}

func assertNoNotificationState(t *testing.T, h *harness, eventID string) {
	t.Helper()
	if got := h.count(t, "SELECT count(*) FROM notification.notification_logs WHERE event_id = "+sqlLiteral(eventID)); got != 0 {
		t.Errorf("notification_logs rows for %s = %d, want 0", eventID, got)
	}
	if got := h.count(t, "SELECT count(*) FROM notification.processed_events WHERE consumer = "+
		sqlLiteral(consumerName)+" AND event_id = "+sqlLiteral(eventID)); got != 0 {
		t.Errorf("processed_events rows for %s = %d, want 0", eventID, got)
	}
}

func waitForDeadLetter(t *testing.T, h *harness, queue, eventID string, timeout time.Duration) dlqMessage {
	t.Helper()

	var found *dlqMessage
	err := waitFor(timeout, "event "+eventID+" in "+queue+".dlq", func() error {
		messages := h.dlqPeekQueue(t, queue)
		for index := range messages {
			if strings.Contains(messages[index].Payload, eventID) {
				found = &messages[index]
				return nil
			}
		}
		return fmt.Errorf("event %s is not in %s.dlq yet", eventID, queue)
	})
	if err != nil {
		t.Fatal(err)
	}
	return *found
}

func TestParcelCreatedDomainEventSendsEmailAndPersists(t *testing.T) {
	h := newHarness(t)
	requireStack(t, h)

	customerID, email := h.registerCustomer(t)
	eventID := h.newUUID(t)
	parcelID := h.newUUID(t)
	tracking := "SDK" + strings.ToUpper(h.runID(t)[:8])
	h.trackEvent(eventID)

	body := domainEnvelope(t, eventID, "parcel.created.v1", domainCreatedData(customerID, parcelID, tracking))
	publishDomain(t, h, eventID, body, h.parcelCreatedRoutingKey)

	messages := h.waitForMailpitSubject(t, email, tracking, 1, 30*time.Second)
	if len(messages) != 1 {
		t.Fatalf("got %d parcel-created emails, want 1", len(messages))
	}
	message, err := h.mailpitMessage(messages[0].ID)
	if err != nil {
		t.Fatalf("read email: %v", err)
	}
	if len(message.To) != 1 || message.To[0].Address != email {
		t.Errorf("recipients = %+v, want the registered customer", message.To)
	}
	for _, want := range []string{"Domain Flow Customer", "Current status: Created", "Tracking number: " + tracking} {
		if !strings.Contains(message.Text, want) {
			t.Errorf("email text does not contain %q:\n%s", want, message.Text)
		}
	}
	if !strings.Contains(message.HTML, tracking) {
		t.Error("email HTML does not contain the tracking number")
	}

	assertNotificationState(t, h, eventID, email, tracking)
}

func TestParcelStatusChangedDomainEventSendsStatusSpecificEmail(t *testing.T) {
	h := newHarness(t)
	requireStack(t, h)

	customerID, email := h.registerCustomer(t)
	eventID := h.newUUID(t)
	parcelID := h.newUUID(t)
	tracking := "SDK" + strings.ToUpper(h.runID(t)[:8])
	h.trackEvent(eventID)

	body := domainEnvelope(t, eventID, "parcel.status_changed.v1",
		domainStatusData(customerID, parcelID, tracking, "PICKED_UP", "IN_TRANSIT", ""))
	publishDomain(t, h, eventID, body, h.parcelStatusRoutingKey)

	messages := h.waitForMailpitSubject(t, email, tracking, 1, 30*time.Second)
	if len(messages) != 1 {
		t.Fatalf("got %d status emails, want 1", len(messages))
	}
	message, err := h.mailpitMessage(messages[0].ID)
	if err != nil {
		t.Fatalf("read email: %v", err)
	}
	for _, want := range []string{"Current status: In transit", "Previous status: Picked up", "Tracking number: " + tracking} {
		if !strings.Contains(message.Text, want) {
			t.Errorf("email text does not contain %q:\n%s", want, message.Text)
		}
	}

	assertNotificationState(t, h, eventID, email, tracking)
}

func TestParcelDomainEventDuplicateDoesNotSendSecondEmail(t *testing.T) {
	h := newHarness(t)
	requireStack(t, h)

	customerID, email := h.registerCustomer(t)
	eventID := h.newUUID(t)
	parcelID := h.newUUID(t)
	tracking := "SDK" + strings.ToUpper(h.runID(t)[:8])
	h.trackEvent(eventID)

	body := domainEnvelope(t, eventID, "parcel.created.v1", domainCreatedData(customerID, parcelID, tracking))
	publishDomain(t, h, eventID, body, h.parcelCreatedRoutingKey)
	h.waitForMailpitSubject(t, email, tracking, 1, 30*time.Second)
	assertNotificationState(t, h, eventID, email, tracking)
	sentAtBefore := h.scalar(t, "SELECT sent_at FROM notification.notification_logs WHERE event_id = "+
		sqlLiteral(eventID)+" AND recipient = "+sqlLiteral(email))

	publishDomain(t, h, eventID, body, h.parcelCreatedRoutingKey)
	h.waitForAlreadyProcessedLog(t, eventID, 15*time.Second)

	messages, err := h.mailpitSearch(email)
	if err != nil {
		t.Fatalf("search Mailpit: %v", err)
	}
	parcelMessages := 0
	for _, message := range messages {
		if strings.Contains(message.Subject, tracking) {
			parcelMessages++
		}
	}
	if parcelMessages != 1 {
		t.Errorf("parcel emails = %d after duplicate, want 1", parcelMessages)
	}
	if got := h.count(t, "SELECT count(*) FROM notification.notification_logs WHERE event_id = "+sqlLiteral(eventID)); got != 1 {
		t.Errorf("notification_logs rows = %d, want 1", got)
	}
	if got := h.count(t, "SELECT count(*) FROM notification.processed_events WHERE consumer = "+
		sqlLiteral(consumerName)+" AND event_id = "+sqlLiteral(eventID)); got != 1 {
		t.Errorf("processed_events rows = %d, want 1", got)
	}
	sentAtAfter := h.scalar(t, "SELECT sent_at FROM notification.notification_logs WHERE event_id = "+
		sqlLiteral(eventID)+" AND recipient = "+sqlLiteral(email))
	if sentAtAfter != sentAtBefore {
		t.Errorf("sent_at changed from %q to %q; the duplicate was reprocessed", sentAtBefore, sentAtAfter)
	}
}

func TestParcelDomainEventMerchantOwnedSendsToApprovedContact(t *testing.T) {
	h := newHarness(t)
	requireStack(t, h)

	email := "merchant-" + h.runID(t) + "@example.test"
	merchantID := h.createMerchant(t, "APPROVED", email)
	eventID := h.newUUID(t)
	parcelID := h.newUUID(t)
	tracking := "SDN" + strings.ToUpper(h.runID(t)[:8])
	h.trackEvent(eventID)

	data := domainCreatedData("", parcelID, tracking)
	delete(data, "customer_user_id")
	data["merchant_id"] = merchantID
	body := domainEnvelope(t, eventID, "parcel.created.v1", data)
	publishDomain(t, h, eventID, body, h.parcelCreatedRoutingKey)

	messages := h.waitForMailpitSubject(t, email, tracking, 1, 30*time.Second)
	if len(messages) != 1 {
		t.Fatalf("got %d merchant emails, want 1", len(messages))
	}
	message, err := h.mailpitMessage(messages[0].ID)
	if err != nil {
		t.Fatalf("read email: %v", err)
	}
	for _, want := range []string{"Tracking number: " + tracking, "Current status: Created"} {
		if !strings.Contains(message.Text, want) {
			t.Errorf("email text does not contain %q:\n%s", want, message.Text)
		}
	}
	assertNotificationState(t, h, eventID, email, tracking)
}

func TestParcelDomainEventMerchantOwnedSuspendedIsDeadLettered(t *testing.T) {
	h := newHarness(t)
	requireStack(t, h)

	email := "suspended-" + h.runID(t) + "@example.test"
	merchantID := h.createMerchant(t, "SUSPENDED", email)
	eventID := h.newUUID(t)
	parcelID := h.newUUID(t)
	tracking := "SDN" + strings.ToUpper(h.runID(t)[:8])
	h.trackEvent(eventID)

	data := domainStatusData("", parcelID, tracking, "CREATED", "PICKED_UP", "")
	delete(data, "customer_user_id")
	data["merchant_id"] = merchantID
	body := domainEnvelope(t, eventID, "parcel.status_changed.v1", data)
	publishDomain(t, h, eventID, body, h.parcelStatusRoutingKey)

	deadLetter := waitForDeadLetter(t, h, h.parcelStatusQueue, eventID, 20*time.Second)
	reason, _ := deadLetter.Properties.Headers["x-swiftdrop-failure-reason"].(string)
	if reason != "permanent" {
		t.Errorf("failure reason = %q, want permanent", reason)
	}
	assertNoNotificationState(t, h, eventID)
}

func TestParcelDomainEventInvalidIsDeadLettered(t *testing.T) {
	h := newHarness(t)
	requireStack(t, h)

	eventID := h.newUUID(t)
	data := domainCreatedData(h.newUUID(t), h.newUUID(t), "SDK7M2Q9XP4")
	delete(data, "parcel_id")
	h.trackEvent(eventID)
	body := domainEnvelope(t, eventID, "parcel.created.v1", data)
	publishDomain(t, h, eventID, body, h.parcelCreatedRoutingKey)

	deadLetter := waitForDeadLetter(t, h, h.parcelCreatedQueue, eventID, 20*time.Second)
	reason, _ := deadLetter.Properties.Headers["x-swiftdrop-failure-reason"].(string)
	if reason != "permanent" {
		t.Errorf("failure reason = %q, want permanent", reason)
	}
	assertNoNotificationState(t, h, eventID)
}

func TestParcelDomainEventForeignProducerIsDeadLettered(t *testing.T) {
	h := newHarness(t)
	requireStack(t, h)

	eventID := h.newUUID(t)
	parcelID := h.newUUID(t)
	tracking := "SDK" + strings.ToUpper(h.runID(t)[:8])
	h.trackEvent(eventID)

	body := domainEnvelopeWithProducer(t, eventID, "parcel.created.v1", "identity", domainCreatedData(h.newUUID(t), parcelID, tracking))
	publishDomain(t, h, eventID, body, h.parcelCreatedRoutingKey)

	deadLetter := waitForDeadLetter(t, h, h.parcelCreatedQueue, eventID, 20*time.Second)
	reason, _ := deadLetter.Properties.Headers["x-swiftdrop-failure-reason"].(string)
	if reason != "permanent" {
		t.Errorf("failure reason = %q, want permanent", reason)
	}
	assertNoNotificationState(t, h, eventID)
}

func TestParcelDomainEventIdentityNotFoundIsDeadLettered(t *testing.T) {
	h := newHarness(t)
	requireStack(t, h)

	eventID := h.newUUID(t)
	parcelID := h.newUUID(t)
	tracking := "SDK" + strings.ToUpper(h.runID(t)[:8])
	h.trackEvent(eventID)

	body := domainEnvelope(t, eventID, "parcel.created.v1", domainCreatedData(h.newUUID(t), parcelID, tracking))
	publishDomain(t, h, eventID, body, h.parcelCreatedRoutingKey)

	deadLetter := waitForDeadLetter(t, h, h.parcelCreatedQueue, eventID, 20*time.Second)
	reason, _ := deadLetter.Properties.Headers["x-swiftdrop-failure-reason"].(string)
	if reason != "permanent" {
		t.Errorf("failure reason = %q, want permanent", reason)
	}
	assertNoNotificationState(t, h, eventID)
}

func TestParcelDomainEventSMTPFailureRecovers(t *testing.T) {
	h := newHarness(t)
	requireStack(t, h)

	customerID, email := h.registerCustomer(t)
	eventID := h.newUUID(t)
	parcelID := h.newUUID(t)
	tracking := "SDK" + strings.ToUpper(h.runID(t)[:8])
	h.trackEvent(eventID)

	t.Cleanup(h.startMailpitBestEffort)
	h.stopMailpit(t)

	body := domainEnvelope(t, eventID, "parcel.created.v1", domainCreatedData(customerID, parcelID, tracking))
	publishDomain(t, h, eventID, body, h.parcelCreatedRoutingKey)

	h.waitForStatus(t, eventID, email, "PENDING", 20*time.Second)
	if got := h.count(t, "SELECT count(*) FROM notification.processed_events WHERE consumer = "+
		sqlLiteral(consumerName)+" AND event_id = "+sqlLiteral(eventID)); got != 0 {
		t.Errorf("processed_events rows = %d after SMTP failure, want 0", got)
	}

	h.startMailpit(t)
	h.waitForStatus(t, eventID, email, "SENT", h.retryWait)
	h.waitForProcessed(t, eventID, 1, h.retryWait)

	messages := h.waitForMailpitSubject(t, email, tracking, 1, 30*time.Second)
	if len(messages) != 1 {
		t.Errorf("parcel emails = %d after recovery, want 1", len(messages))
	}
}

func TestParcelDomainEventIdentityUnavailableIsDeadLettered(t *testing.T) {
	h := newHarness(t)
	requireStack(t, h)

	eventID := h.newUUID(t)
	parcelID := h.newUUID(t)
	tracking := "SDK" + strings.ToUpper(h.runID(t)[:8])
	h.trackEvent(eventID)

	t.Cleanup(h.startIdentityBestEffort)
	h.stopIdentity(t)

	body := domainEnvelope(t, eventID, "parcel.created.v1", domainCreatedData(h.newUUID(t), parcelID, tracking))
	publishDomain(t, h, eventID, body, h.parcelCreatedRoutingKey)

	deadLetter := waitForDeadLetter(t, h, h.parcelCreatedQueue, eventID, 60*time.Second)
	reason, _ := deadLetter.Properties.Headers["x-swiftdrop-failure-reason"].(string)
	if reason != "retry_exhausted" {
		t.Errorf("failure reason = %q, want retry_exhausted", reason)
	}
	assertNoNotificationState(t, h, eventID)

	h.startIdentity(t)
}
