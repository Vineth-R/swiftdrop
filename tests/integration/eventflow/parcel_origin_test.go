//go:build integration

package eventflow

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	notificationevents "github.com/Vineth-R/swiftdrop/apps/services/notification-worker/eventtest"
	parcelevents "github.com/Vineth-R/swiftdrop/apps/services/parcel/eventtest"
	"github.com/Vineth-R/swiftdrop/packages/go-common/emails"
	"github.com/Vineth-R/swiftdrop/packages/go-common/messaging"
)

// TestParcelCreationAndAssignmentPublishVersionedEvents proves the customer and
// dispatcher origins of the event chain: the real parcel creation transaction
// writes parcel.created, the real assignment transaction writes
// parcel.assigned, and the real relay publishes both as versioned events. The
// created branch also drives a real notification email to the customer.
func TestParcelCreationAndAssignmentPublishVersionedEvents(t *testing.T) {
	rabbitURL := os.Getenv("RABBITMQ_URL")
	if rabbitURL == "" {
		t.Skip("RABBITMQ_URL is not set; skipping event-flow integration tests")
	}
	pool, _ := newIsolatedDatabase(t)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ctx, cancel := context.WithCancel(t.Context())

	runID := randomHex(6)
	exchange := "eventflow-origin." + runID
	notificationQueue := "eventflow-origin." + runID + ".notification"
	assignedQueue := "eventflow-origin." + runID + ".assigned"
	t.Cleanup(func() {
		cancel()
		deleteTopology(t, rabbitURL, exchange, notificationQueue, assignedQueue)
	})

	customerID := newUUID(t, pool)
	customerEmail := "origin-customer-" + runID + "@example.test"
	identityServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"data":{"fullName":"Origin Customer","email":%q,"phone":""}}`, customerEmail)
	}))
	defer identityServer.Close()

	mail := &captureMailer{}
	notification, err := notificationevents.NewHandler(
		logger, mail, pool, "eventflow", identityServer.URL, identityServer.URL, testInternalToken,
		emails.Site{BaseURL: "http://localhost:8080"},
	)
	if err != nil {
		t.Fatalf("notification handler: %v", err)
	}
	startConsumerWithRoutingKey(t, ctx, rabbitURL, exchange, notificationQueue, "parcel.created.v1", notification, logger)
	waitForConsumer(t, rabbitURL, notificationQueue)

	// No product consumer handles parcel.assigned.v1 yet (the realtime service
	// is status-only per API v1.1 section 9.3), so bind a sink to prove the
	// event is published and routable rather than unroutable.
	assignedSink := &recordingHandler{}
	startConsumerWithRoutingKey(t, ctx, rabbitURL, exchange, assignedQueue, "parcel.assigned.v1", assignedSink, logger)
	waitForConsumer(t, rabbitURL, assignedQueue)

	// Real parcel creation transaction: parcel + initial history + parcel.created.
	tracking := "SDK" + strings.ToUpper(randomHex(4))
	parcel, err := parcelevents.NewParcelStore(pool).Create(ctx, parcelevents.CreateParcelParams{
		ID:              randomUUID(),
		TrackingNumber:  tracking,
		CustomerUserID:  &customerID,
		SenderName:      "Origin Sender",
		SenderPhone:     "+94770000001",
		PickupAddress:   "1 Origin Road",
		PickupCity:      "Colombo",
		PickupZoneID:    randomUUID(),
		RecipientName:   "Origin Recipient",
		RecipientPhone:  "+94770000002",
		DropoffAddress:  "2 Destination Road",
		DropoffCity:     "Kandy",
		DropoffZoneID:   randomUUID(),
		WeightKG:        numericValue(2, 0),
		SizeCategory:    parcelevents.SizeSmall,
		PriceAmount:     numericValue(55000, -2),
		Currency:        "LKR",
		PriceBreakdown:  json.RawMessage(`{"base_fee":450,"weight_charge":100,"size_surcharge":0}`),
		CreatedByUserID: customerID,
	}, "CUSTOMER")
	if err != nil {
		t.Fatalf("create parcel: %v", err)
	}

	// Real dispatch assignment transaction: assignment + parcel.assigned.
	courierID := newUUID(t, pool)
	dispatcherID := newUUID(t, pool)
	if _, err := parcelevents.NewDispatchRepository(pool).AssignCourier(ctx, parcelevents.AssignInput{
		ParcelID: parcel.ID, CourierID: courierID, Reason: "origin flow", ActorUserID: dispatcherID,
	}); err != nil {
		t.Fatalf("assign courier: %v", err)
	}

	amqpPublisher := parcelevents.NewAMQPPublisher(rabbitURL, exchange)
	defer func() { _ = amqpPublisher.Close() }()
	publisher := &recordingPublisher{inner: amqpPublisher}
	relay := parcelevents.NewRelay(parcelevents.NewSQLStore(pool), publisher, logger, parcelevents.RelayOptions{
		Exchange: exchange, BatchSize: 10, PollInterval: time.Second,
	})
	waitUntil(t, eventWaitTimeout, "created and assigned events published", func() error {
		if _, err := relay.PollOnce(ctx); err != nil {
			return err
		}
		if len(publisher.snapshot()) < 2 {
			return fmt.Errorf("published %d events", len(publisher.snapshot()))
		}
		return nil
	})

	byKey := publishedByKey(t, publisher.snapshot())
	created, ok := byKey["parcel.created.v1"]
	if !ok {
		t.Fatalf("no parcel.created.v1 publication: keys %v", mapKeys(byKey))
	}
	assigned, ok := byKey["parcel.assigned.v1"]
	if !ok {
		t.Fatalf("no parcel.assigned.v1 publication: keys %v", mapKeys(byKey))
	}
	if created.Type != "parcel.created.v1" || created.Version != 1 || created.Producer != "parcel" {
		t.Errorf("created envelope = %s v%d %s", created.Type, created.Version, created.Producer)
	}
	if assigned.Type != "parcel.assigned.v1" || assigned.Version != 1 || assigned.Producer != "parcel" {
		t.Errorf("assigned envelope = %s v%d %s", assigned.Type, assigned.Version, assigned.Producer)
	}

	var createdData, assignedData map[string]any
	if err := json.Unmarshal(created.Data, &createdData); err != nil {
		t.Fatalf("decode created data: %v", err)
	}
	if createdData["parcel_id"] != parcel.ID || createdData["tracking_number"] != tracking || createdData["status"] != "CREATED" || createdData["customer_user_id"] != customerID {
		t.Errorf("created data = %v", createdData)
	}
	if err := json.Unmarshal(assigned.Data, &assignedData); err != nil {
		t.Fatalf("decode assigned data: %v", err)
	}
	if assignedData["parcel_id"] != parcel.ID || assignedData["courier_id"] != courierID || assignedData["assigned_by"] != dispatcherID {
		t.Errorf("assigned data = %v", assignedData)
	}

	// The assigned event is routed to a bound queue (it is not unroutable).
	waitUntil(t, eventWaitTimeout, "assigned event delivered to a bound queue", func() error {
		ids := assignedSink.ids()
		if len(ids) < 1 {
			return fmt.Errorf("sink received %d assigned events", len(ids))
		}
		if ids[0] != assigned.ID {
			return fmt.Errorf("sink received event %s, want %s", ids[0], assigned.ID)
		}
		return nil
	})

	// The created branch drives a real notification email to the customer.
	waitUntil(t, eventWaitTimeout, "created notification email", func() error {
		if mail.recipients()[customerEmail] < 1 {
			return fmt.Errorf("sent %d emails to %s", mail.recipients()[customerEmail], customerEmail)
		}
		return nil
	})
	waitUntil(t, eventWaitTimeout, "created notification log committed", func() error {
		if got := scalar(t, pool, "SELECT count(*) FROM notification.notification_logs WHERE event_id='"+created.ID+"' AND status='SENT'"); got != "1" {
			return fmt.Errorf("SENT logs for created event = %s", got)
		}
		return nil
	})

	// Both outbox rows are terminal and no claim is left behind.
	if got := scalar(t, pool, "SELECT count(*) FROM parcel.outbox_events WHERE status='PUBLISHED'"); got != "2" {
		t.Errorf("PUBLISHED outbox rows = %s, want 2", got)
	}
	if got := scalar(t, pool, "SELECT count(*) FROM parcel.outbox_events WHERE claim_token IS NOT NULL"); got != "0" {
		t.Errorf("leftover claimed rows = %s, want 0", got)
	}
}

func numericValue(value int64, exponent int32) pgtype.Numeric {
	return pgtype.Numeric{Int: big.NewInt(value), Exp: exponent, Valid: true}
}

// recordingHandler is a sink consumer that records the event IDs it receives.
type recordingHandler struct {
	mu       sync.Mutex
	eventIDs []string
}

func (h *recordingHandler) Handle(_ context.Context, message messaging.Message) error {
	var envelope messaging.Envelope
	if err := json.Unmarshal(message.Body, &envelope); err != nil {
		return nil
	}
	h.mu.Lock()
	h.eventIDs = append(h.eventIDs, envelope.ID)
	h.mu.Unlock()
	return nil
}

func (h *recordingHandler) ids() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]string(nil), h.eventIDs...)
}

func publishedByKey(t *testing.T, items []capturedPublish) map[string]messaging.Envelope {
	t.Helper()
	byKey := make(map[string]messaging.Envelope, len(items))
	for _, item := range items {
		var envelope messaging.Envelope
		if err := json.Unmarshal(item.body, &envelope); err != nil {
			t.Fatalf("decode published envelope: %v", err)
		}
		if err := envelope.Validate(); err != nil {
			t.Fatalf("published envelope is invalid: %v", err)
		}
		if item.key != envelope.Type {
			t.Errorf("routing key %q does not match event type %q", item.key, envelope.Type)
		}
		byKey[item.key] = envelope
	}
	return byKey
}

func mapKeys(values map[string]messaging.Envelope) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	return keys
}
