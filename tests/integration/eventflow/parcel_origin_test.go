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
	realtimeevents "github.com/Vineth-R/swiftdrop/apps/services/realtime/eventtest"
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

	// The real realtime assignment consumer binds parcel.assigned.v1 and turns
	// it into a privacy-safe dispatch notice.
	assignmentBroadcaster := &recordingAssignmentBroadcaster{}
	assignedConsumer := realtimeevents.NewAssignmentConsumer(assignmentBroadcaster)
	startConsumerWithRoutingKey(t, ctx, rabbitURL, exchange, assignedQueue, "parcel.assigned.v1", assignedConsumer, logger)
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

	// The assignment event is routed to the real consumer, which emits only the
	// tracking number and event time. The courier and assigner IDs never reach a
	// client.
	waitUntil(t, eventWaitTimeout, "assignment notice delivered", func() error {
		notices := assignmentBroadcaster.updates()
		if len(notices) < 1 {
			return fmt.Errorf("assignment broadcaster received %d notices", len(notices))
		}
		if notices[0].TrackingNumber != tracking {
			return fmt.Errorf("assignment notice tracking = %s, want %s", notices[0].TrackingNumber, tracking)
		}
		return nil
	})
	if body := string(assignmentBroadcaster.updates()[0].Message()); strings.Contains(body, courierID) || strings.Contains(body, dispatcherID) {
		t.Errorf("assignment notice leaks an identity: %s", body)
	}

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

// recordingAssignmentBroadcaster records the assignment notices the real
// consumer emits.
type recordingAssignmentBroadcaster struct {
	mu    sync.Mutex
	items []realtimeevents.AssignmentUpdate
}

func (b *recordingAssignmentBroadcaster) BroadcastAssignment(update realtimeevents.AssignmentUpdate) int {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.items = append(b.items, update)
	return 1
}

func (b *recordingAssignmentBroadcaster) updates() []realtimeevents.AssignmentUpdate {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]realtimeevents.AssignmentUpdate(nil), b.items...)
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
