//go:build notificationflow

package notificationflow

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

// TestNotificationWorkerDeliversValidEvent proves one contract-valid event is
// consumed, delivered through the real SMTP path, persisted as SENT, and
// recorded in processed_events.
func TestNotificationWorkerDeliversValidEvent(t *testing.T) {
	h := newHarness(t)
	requireStack(t, h)

	runID := h.runID(t)
	eventID := h.newUUID(t)
	parcelID := h.newUUID(t)
	recipient := "notificationflow." + runID + "@example.test"
	template := "Notification flow test " + runID
	trackingNumber := "SDK" + strings.ToUpper(runID[:8])
	h.trackEvent(eventID)
	h.trackRecipient(recipient)

	h.publish(t, eventID, validEvent(t, eventID, parcelID, recipient, template, map[string]string{
		"trackingNumber": trackingNumber,
		"status":         "IN_TRANSIT",
	}))

	messages := h.waitForMailpit(t, recipient, 1, 30*time.Second)
	if len(messages) != 1 {
		t.Fatalf("got %d emails for %s, want exactly 1", len(messages), recipient)
	}

	message, err := h.mailpitMessage(messages[0].ID)
	if err != nil {
		t.Fatalf("read email %s: %v", messages[0].ID, err)
	}
	if message.Subject != template {
		t.Errorf("email subject = %q, want %q", message.Subject, template)
	}
	if len(message.To) != 1 || message.To[0].Address != recipient {
		t.Errorf("email recipients = %+v, want %s", message.To, recipient)
	}
	for _, want := range []string{template, "trackingNumber: " + trackingNumber, "status: IN_TRANSIT"} {
		if !strings.Contains(message.Text, want) {
			t.Errorf("email body does not contain %q:\n%s", want, message.Text)
		}
	}

	h.waitForStatus(t, eventID, recipient, "SENT", 30*time.Second)
	h.waitForProcessed(t, eventID, 1, 30*time.Second)

	if got := h.count(t, "SELECT count(*) FROM notification.notification_logs WHERE event_id = "+sqlLiteral(eventID)); got != 1 {
		t.Errorf("notification_logs rows for %s = %d, want 1", eventID, got)
	}
	if got := h.scalar(t, "SELECT sent_at IS NOT NULL FROM notification.notification_logs WHERE event_id = "+
		sqlLiteral(eventID)+" AND recipient = "+sqlLiteral(recipient)); got != "t" {
		t.Errorf("sent_at is NULL for a delivered notification")
	}
	if got := h.count(t, "SELECT count(*) FROM notification.processed_events WHERE consumer = "+
		sqlLiteral(consumerName)+" AND event_id = "+sqlLiteral(eventID)); got != 1 {
		t.Errorf("processed_events rows for %s = %d, want 1", eventID, got)
	}
}

// TestNotificationWorkerDeduplicatesRepeatedEvent proves the existing
// processed_events check prevents a second email when the same event is
// delivered again after the first delivery has fully completed.
func TestNotificationWorkerDeduplicatesRepeatedEvent(t *testing.T) {
	h := newHarness(t)
	requireStack(t, h)

	runID := h.runID(t)
	eventID := h.newUUID(t)
	parcelID := h.newUUID(t)
	recipient := "notificationflow.duplicate." + runID + "@example.test"
	template := "Notification flow duplicate " + runID
	h.trackEvent(eventID)
	h.trackRecipient(recipient)

	body := validEvent(t, eventID, parcelID, recipient, template, map[string]string{
		"trackingNumber": "SDK" + strings.ToUpper(runID[:8]),
	})

	h.publish(t, eventID, body)
	h.waitForStatus(t, eventID, recipient, "SENT", 30*time.Second)
	h.waitForProcessed(t, eventID, 1, 30*time.Second)
	if got := h.mailpitCount(t, recipient); got != 1 {
		t.Fatalf("first delivery produced %d emails, want 1", got)
	}
	sentAtBefore := h.scalar(t, "SELECT sent_at FROM notification.notification_logs WHERE event_id = "+
		sqlLiteral(eventID)+" AND recipient = "+sqlLiteral(recipient))

	// Republish the identical envelope, as a broker redelivery would.
	h.publish(t, eventID, body)
	h.waitQueueDrained(t, h.queue, 15*time.Second)
	time.Sleep(500 * time.Millisecond)

	if got := h.mailpitCount(t, recipient); got != 1 {
		t.Errorf("duplicate delivery produced %d emails, want 1", got)
	}
	if got := h.count(t, "SELECT count(*) FROM notification.notification_logs WHERE event_id = "+sqlLiteral(eventID)); got != 1 {
		t.Errorf("notification_logs rows for %s = %d, want 1", eventID, got)
	}
	if got := h.count(t, "SELECT count(*) FROM notification.processed_events WHERE consumer = "+
		sqlLiteral(consumerName)+" AND event_id = "+sqlLiteral(eventID)); got != 1 {
		t.Errorf("processed_events rows for %s = %d, want 1", eventID, got)
	}
	sentAtAfter := h.scalar(t, "SELECT sent_at FROM notification.notification_logs WHERE event_id = "+
		sqlLiteral(eventID)+" AND recipient = "+sqlLiteral(recipient))
	if sentAtAfter != sentAtBefore {
		t.Errorf("sent_at changed from %q to %q; the duplicate was reprocessed", sentAtBefore, sentAtAfter)
	}
}

// TestNotificationWorkerRejectsInvalidEvents proves malformed and semantically
// invalid messages are dead-lettered as permanent failures without producing
// email or delivery state, and that the worker still processes a valid event
// afterwards.
func TestNotificationWorkerRejectsInvalidEvents(t *testing.T) {
	h := newHarness(t)
	requireStack(t, h)

	cases := []struct {
		name string
		body func(t *testing.T, eventID, parcelID, recipient string) string
	}{
		{
			name: "malformed JSON",
			body: func(_ *testing.T, eventID, _, recipient string) string {
				return fmt.Sprintf(`{"event_id":%q,"event_type":"notification.requested.v1","data":{"recipient":%q`,
					eventID, recipient)
			},
		},
		{
			name: "unsupported event type",
			body: func(t *testing.T, eventID, parcelID, recipient string) string {
				return validEventWithType(t, eventID, parcelID, recipient,
					"Notification flow unsupported "+eventID, "notification.requested.v2", 2)
			},
		},
		{
			name: "invalid notification data",
			body: func(t *testing.T, eventID, _, recipient string) string {
				return validEventMissingParcel(t, eventID, recipient)
			},
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			eventID := h.newUUID(t)
			parcelID := h.newUUID(t)
			recipient := "notificationflow.invalid." + h.runID(t) + "@example.test"
			h.trackEvent(eventID)
			h.trackRecipient(recipient)

			before, err := h.queueInfo(h.queue + ".dlq")
			if err != nil {
				t.Fatalf("read dead-letter queue before publish: %v", err)
			}

			h.publish(t, eventID, testCase.body(t, eventID, parcelID, recipient))

			err = waitFor(20*time.Second, "event "+eventID+" to be dead-lettered", func() error {
				after, err := h.queueInfo(h.queue + ".dlq")
				if err != nil {
					return err
				}
				if after.Messages < before.Messages+1 {
					return fmt.Errorf("dead-letter queue has %d messages, want at least %d", after.Messages, before.Messages+1)
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}

			var found *dlqMessage
			peeked := h.dlqPeek(t)
			for index := range peeked {
				if strings.Contains(peeked[index].Payload, eventID) {
					found = &peeked[index]
					break
				}
			}
			if found == nil {
				t.Fatalf("dead-letter queue does not contain event %s", eventID)
			}
			if found.Exchange != h.exchange+".dlx" {
				t.Errorf("dead-letter exchange = %q, want %q", found.Exchange, h.exchange+".dlx")
			}
			if found.RoutingKey != h.queue+".dlq" {
				t.Errorf("dead-letter routing key = %q, want %q", found.RoutingKey, h.queue+".dlq")
			}
			reason, _ := found.Properties.Headers["x-swiftdrop-failure-reason"].(string)
			if reason != "permanent" {
				t.Errorf("failure reason = %q, want %q", reason, "permanent")
			}

			if got := h.mailpitCount(t, recipient); got != 0 {
				t.Errorf("invalid event produced %d emails, want 0", got)
			}
			if got := h.count(t, "SELECT count(*) FROM notification.notification_logs WHERE event_id = "+sqlLiteral(eventID)); got != 0 {
				t.Errorf("notification_logs rows for invalid event %s = %d, want 0", eventID, got)
			}
			if got := h.count(t, "SELECT count(*) FROM notification.processed_events WHERE consumer = "+
				sqlLiteral(consumerName)+" AND event_id = "+sqlLiteral(eventID)); got != 0 {
				t.Errorf("processed_events rows for invalid event %s = %d, want 0", eventID, got)
			}
		})
	}

	// The consumer must still be alive and able to process a valid event.
	runID := h.runID(t)
	eventID := h.newUUID(t)
	parcelID := h.newUUID(t)
	recipient := "notificationflow.afterinvalid." + runID + "@example.test"
	h.trackEvent(eventID)
	h.trackRecipient(recipient)

	h.publish(t, eventID, validEvent(t, eventID, parcelID, recipient,
		"Notification flow after invalid "+runID, nil))
	h.waitForStatus(t, eventID, recipient, "SENT", 30*time.Second)
	h.waitForProcessed(t, eventID, 1, 30*time.Second)
}

// TestNotificationWorkerSurvivesSMTPFailure proves a real SMTP outage is
// retryable, is not recorded as processed, does not crash the worker, and
// recovers once the SMTP service returns.
func TestNotificationWorkerSurvivesSMTPFailure(t *testing.T) {
	h := newHarness(t)
	requireStack(t, h)

	eventID := h.newUUID(t)
	parcelID := h.newUUID(t)
	recipient := "notificationflow.smtp." + h.runID(t) + "@example.test"
	h.trackEvent(eventID)
	h.trackRecipient(recipient)

	// Register the restart before stopping so a later failure still restores
	// the shared Mailpit service. Cleanups run last-in-first-out, so this runs
	// before the harness cleanup that deletes Mailpit messages.
	t.Cleanup(h.startMailpitBestEffort)
	h.stopMailpit(t)

	h.publish(t, eventID, validEvent(t, eventID, parcelID, recipient,
		"Notification flow SMTP failure "+eventID, nil))

	h.waitForStatus(t, eventID, recipient, "PENDING", 20*time.Second)
	if got := h.scalar(t, "SELECT sent_at IS NULL FROM notification.notification_logs WHERE event_id = "+
		sqlLiteral(eventID)+" AND recipient = "+sqlLiteral(recipient)); got != "t" {
		t.Errorf("sent_at is not NULL while the notification is PENDING")
	}
	if got := h.count(t, "SELECT count(*) FROM notification.processed_events WHERE consumer = "+
		sqlLiteral(consumerName)+" AND event_id = "+sqlLiteral(eventID)); got != 0 {
		t.Errorf("event was recorded as processed after an SMTP failure")
	}

	h.startMailpit(t)
	h.waitForStatus(t, eventID, recipient, "SENT", h.retryWait)
	h.waitForProcessed(t, eventID, 1, h.retryWait)

	if got := h.mailpitCount(t, recipient); got != 1 {
		t.Errorf("got %d emails after recovery, want 1", got)
	}
	if info, err := h.queueInfo(h.queue); err != nil || info.Consumers < 1 {
		t.Errorf("worker is not consuming after the SMTP failure: info=%+v err=%v", info, err)
	}
}

func validEvent(t *testing.T, eventID, parcelID, recipient, template string, variables map[string]string) string {
	t.Helper()
	return marshalEvent(t, eventID, eventType, eventVersion, parcelID, recipient, template, variables, true)
}

func validEventWithType(t *testing.T, eventID, parcelID, recipient, template, typ string, version int) string {
	t.Helper()
	return marshalEvent(t, eventID, typ, version, parcelID, recipient, template, nil, true)
}

func validEventMissingParcel(t *testing.T, eventID, recipient string) string {
	t.Helper()
	return marshalEvent(t, eventID, eventType, eventVersion, "", recipient, "Notification flow missing parcel "+eventID, nil, false)
}

func marshalEvent(t *testing.T, eventID, typ string, version int, parcelID, recipient, template string, variables map[string]string, includeParcel bool) string {
	t.Helper()

	data := map[string]any{
		"recipient": recipient,
		"channel":   "email",
		"template":  template,
	}
	if includeParcel {
		data["parcel_id"] = parcelID
	}
	if len(variables) > 0 {
		data["variables"] = variables
	}

	envelope := map[string]any{
		"event_id":       eventID,
		"event_type":     typ,
		"event_version":  version,
		"occurred_at":    time.Now().UTC().Format(time.RFC3339Nano),
		"producer":       "notificationflow-test",
		"correlation_id": "notificationflow-" + eventID,
		"data":           data,
	}

	body, err := json.Marshal(envelope)
	if err != nil {
		t.Fatalf("encode event %s: %v", eventID, err)
	}
	return string(body)
}
