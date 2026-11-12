//go:build integration

package eventflow

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	amqp "github.com/rabbitmq/amqp091-go"

	merchantevents "github.com/Vineth-R/swiftdrop/apps/services/merchant/eventtest"
	notificationevents "github.com/Vineth-R/swiftdrop/apps/services/notification-worker/eventtest"
	parcelevents "github.com/Vineth-R/swiftdrop/apps/services/parcel/eventtest"
	realtimeevents "github.com/Vineth-R/swiftdrop/apps/services/realtime/eventtest"
	webhookevents "github.com/Vineth-R/swiftdrop/apps/services/webhook-worker/eventtest"
	"github.com/Vineth-R/swiftdrop/packages/go-common/emails"
	"github.com/Vineth-R/swiftdrop/packages/go-common/mailer"
	"github.com/Vineth-R/swiftdrop/packages/go-common/messaging"
	"github.com/Vineth-R/swiftdrop/packages/go-common/postgres"
	"github.com/Vineth-R/swiftdrop/packages/webhookdelivery"
)

const (
	testInternalToken = "eventflow-internal-token"
	eventType         = "parcel.status_changed.v1"
	routingKey        = "parcel.status_changed.v1"
)

func TestParcelStatusChangeFlowsToNotificationWebhookAndRealtime(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	rabbitURL := os.Getenv("RABBITMQ_URL")
	if rabbitURL == "" {
		t.Skip("RABBITMQ_URL is not set; skipping event-flow integration tests")
	}
	pool, _ := newIsolatedDatabase(t)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ctx, cancel := context.WithCancel(t.Context())

	runID := randomHex(6)
	exchange := "eventflow." + runID
	queues := struct {
		notification string
		webhook      string
		realtime     string
		merchantSync string
	}{
		notification: "eventflow." + runID + ".notification",
		webhook:      "eventflow." + runID + ".webhook",
		realtime:     "eventflow." + runID + ".realtime",
		merchantSync: "eventflow." + runID + ".merchant-sync",
	}
	t.Cleanup(func() {
		cancel()
		deleteTopology(t, rabbitURL, exchange, queues.notification, queues.webhook, queues.realtime, queues.merchantSync)
	})

	courierRepo := parcelevents.NewCourierRepository(pool)
	customerID := newUUID(t, pool)
	merchantID := newUUID(t, pool)
	merchantEmail := "merchant-" + runID + "@example.test"
	customerEmail := "customer-" + runID + "@example.test"
	insertMerchant(t, pool, merchantID, merchantEmail)

	// The notification worker resolves customers through the real identity
	// client and merchants through the real merchant handler and client.
	var identityFail atomic.Bool
	identityServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if identityFail.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"data":{"fullName":"Flow Customer","email":%q,"phone":""}}`, customerEmail)
	}))
	defer identityServer.Close()
	merchantRouter, err := merchantevents.NewContactRouter(pool, testInternalToken, logger)
	if err != nil {
		t.Fatalf("merchant router: %v", err)
	}
	merchantServer := httptest.NewServer(merchantRouter)
	defer merchantServer.Close()

	mail := &captureMailer{}
	notification, err := notificationevents.NewHandler(
		logger, mail, pool, "eventflow", identityServer.URL, merchantServer.URL, testInternalToken,
		emails.Site{BaseURL: "http://localhost:8080"},
	)
	if err != nil {
		t.Fatalf("notification handler: %v", err)
	}

	webhookMerchantAPI := newFakeMerchantAPI()
	webhookMerchantServer := httptest.NewServer(webhookMerchantAPI)
	defer webhookMerchantServer.Close()
	parcelAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"externalOrderId":"ORDER-EVENTFLOW"}}`))
	}))
	defer parcelAPI.Close()
	sender := &recordingSender{}
	scheduler := &recordingScheduler{}
	webhook := webhookevents.NewHandler(logger, webhookMerchantServer.URL, parcelAPI.URL, testInternalToken, sender, scheduler)

	broadcaster := &recordingBroadcaster{}
	realtime := realtimeevents.NewConsumer(broadcaster)
	merchantSync, err := merchantevents.NewSyncHandler(pool, logger)
	if err != nil {
		t.Fatalf("merchant sync handler: %v", err)
	}

	startConsumer(t, ctx, rabbitURL, exchange, queues.notification, notification, logger)
	startConsumer(t, ctx, rabbitURL, exchange, queues.webhook, webhook, logger)
	startConsumer(t, ctx, rabbitURL, exchange, queues.realtime, realtime, logger)
	startConsumer(t, ctx, rabbitURL, exchange, queues.merchantSync, merchantSync, logger)
	for _, queue := range []string{queues.notification, queues.webhook, queues.realtime, queues.merchantSync} {
		waitForConsumer(t, rabbitURL, queue)
	}

	// A real courier status transaction writes the parcel, history and outbox.
	customerParcel, customerTracking := insertParcel(t, pool, customerID, false)
	if _, err := courierRepo.UpdateStatus(ctx, parcelevents.StatusInput{
		ParcelID: customerParcel, CourierID: courierIDFor(t, pool, customerParcel), Status: parcelevents.StatusPickedUp,
	}); err != nil {
		t.Fatalf("customer status update: %v", err)
	}
	merchantParcel, merchantTracking := insertParcel(t, pool, merchantID, true)
	insertMerchantOrder(t, pool, merchantID, merchantParcel, merchantTracking)
	if _, err := courierRepo.UpdateStatus(ctx, parcelevents.StatusInput{
		ParcelID: merchantParcel, CourierID: courierIDFor(t, pool, merchantParcel), Status: parcelevents.StatusPickedUp,
	}); err != nil {
		t.Fatalf("merchant status update: %v", err)
	}

	// The real relay publishes committed outbox rows through the real AMQP publisher.
	amqpPublisher := parcelevents.NewAMQPPublisher(rabbitURL, exchange)
	defer func() { _ = amqpPublisher.Close() }()
	publisher := &recordingPublisher{inner: amqpPublisher}
	relay := parcelevents.NewRelay(parcelevents.NewSQLStore(pool), publisher, logger, parcelevents.RelayOptions{
		Exchange: exchange, BatchSize: 10, PollInterval: time.Second,
	})
	waitUntil(t, eventWaitTimeout, "two outbox events published", func() error {
		if _, err := relay.PollOnce(ctx); err != nil {
			return err
		}
		captured := publisher.snapshot()
		if len(captured) < 2 {
			return fmt.Errorf("published %d events", len(captured))
		}
		return nil
	})

	envelopes := publisher.envelopes(t)
	if len(envelopes) != 2 {
		t.Fatalf("published %d envelopes, want 2", len(envelopes))
	}
	customerEnvelope, merchantEnvelope := classifyEnvelopes(t, envelopes, customerID, merchantID)
	outboxIDs := outboxEventIDs(t, pool)
	for _, envelope := range []messaging.Envelope{customerEnvelope, merchantEnvelope} {
		if envelope.ID != outboxIDs[envelope.ID] {
			t.Errorf("envelope ID %s does not match an outbox row", envelope.ID)
		}
		if envelope.Type != eventType || envelope.Version != 1 || envelope.Producer != "parcel" {
			t.Errorf("envelope = %s v%d %s", envelope.Type, envelope.Version, envelope.Producer)
		}
	}

	// Notification: one customer email and one merchant contact email.
	waitUntil(t, eventWaitTimeout, "two notification emails", func() error {
		if len(mail.snapshot()) < 2 {
			return fmt.Errorf("sent %d emails", len(mail.snapshot()))
		}
		return nil
	})
	recipients := mail.recipients()
	if recipients[customerEmail] != 1 || recipients[merchantEmail] != 1 {
		t.Fatalf("notification recipients = %v", recipients)
	}
	if got := scalar(t, pool, "SELECT count(*) FROM notification.notification_logs WHERE status='SENT'"); got != "2" {
		t.Errorf("SENT notification logs = %s, want 2", got)
	}
	if got := scalar(t, pool, "SELECT count(*) FROM notification.processed_events"); got != "2" {
		t.Errorf("processed events = %s, want 2", got)
	}
	if got := scalar(t, pool, "SELECT recipient FROM notification.notification_logs WHERE event_id='"+merchantEnvelope.ID+"'"); got != merchantEmail {
		t.Errorf("merchant notification recipient = %q, want %q", got, merchantEmail)
	}

	// Webhook worker: one signed delivery for the merchant-owned event only.
	waitUntil(t, eventWaitTimeout, "webhook delivery", func() error {
		if len(sender.calls()) < 1 {
			return fmt.Errorf("sender calls = %d", len(sender.calls()))
		}
		return nil
	})
	deliveries := sender.calls()
	if len(deliveries) != 1 {
		t.Fatalf("webhook deliveries = %d, want 1", len(deliveries))
	}
	if deliveries[0].event.ID != merchantEnvelope.ID || deliveries[0].target.Secret != "whsec_eventflow" {
		t.Errorf("webhook delivery = %+v", deliveries[0])
	}
	if attempts := webhookMerchantAPI.attempts(); len(attempts) != 1 || attempts[0].EventID != merchantEnvelope.ID {
		t.Errorf("recorded webhook attempts = %+v", attempts)
	}

	// Realtime: public tracking projection only.
	waitUntil(t, eventWaitTimeout, "realtime updates", func() error {
		if len(broadcaster.updates()) < 2 {
			return fmt.Errorf("realtime updates = %d", len(broadcaster.updates()))
		}
		return nil
	})
	updates := broadcaster.updates()
	trackings := map[string]bool{}
	for _, update := range updates {
		trackings[update.TrackingNumber] = true
	}
	if !trackings[customerTracking] || !trackings[merchantTracking] {
		t.Errorf("realtime trackings = %v", trackings)
	}

	// Merchant integration: the real merchant.sync handler updates the order
	// snapshot and timeline exactly once.
	waitUntil(t, eventWaitTimeout, "merchant order status synchronization", func() error {
		status := scalar(t, pool, "SELECT last_known_status FROM merchant.merchant_orders WHERE parcel_id='"+merchantParcel+"'")
		if status != "PICKED_UP" {
			return fmt.Errorf("merchant order status = %q", status)
		}
		return nil
	})
	if got := scalar(t, pool, "SELECT count(*) FROM merchant.merchant_order_status_events WHERE event_id='"+merchantEnvelope.ID+"'"); got != "1" {
		t.Errorf("merchant status events = %s, want 1", got)
	}
	beforeTimeline := scalar(t, pool, "SELECT count(*) FROM merchant.merchant_order_status_events WHERE order_id=(SELECT id FROM merchant.merchant_orders WHERE parcel_id='"+merchantParcel+"')")
	merchantBody, err := json.Marshal(merchantEnvelope)
	if err != nil {
		t.Fatalf("marshal merchant envelope: %v", err)
	}
	if err := merchantSync.Handle(ctx, messaging.Message{Body: merchantBody}); err != nil {
		t.Fatalf("duplicate merchant sync: %v", err)
	}
	afterTimeline := scalar(t, pool, "SELECT count(*) FROM merchant.merchant_order_status_events WHERE order_id=(SELECT id FROM merchant.merchant_orders WHERE parcel_id='"+merchantParcel+"')")
	if afterTimeline != beforeTimeline {
		t.Errorf("merchant timeline grew on duplicate: %s -> %s", beforeTimeline, afterTimeline)
	}

	// Duplicate delivery is idempotent: the merchant event does not send twice.
	if err := notification.Handle(ctx, messaging.Message{Body: merchantBody}); err != nil {
		t.Fatalf("duplicate merchant notification: %v", err)
	}
	if got := mail.recipients()[merchantEmail]; got != 1 {
		t.Errorf("merchant emails after duplicate = %d, want 1", got)
	}
	if got := scalar(t, pool, "SELECT count(*) FROM notification.notification_logs"); got != "2" {
		t.Errorf("notification logs after duplicate = %s, want 2", got)
	}

	// A transient identity failure is retryable and does not send or mark the event.
	identityFail.Store(true)
	retryEnvelope := syntheticCustomerEnvelope(t, customerID)
	retryBody, _ := json.Marshal(retryEnvelope)
	err = notification.Handle(ctx, messaging.Message{Body: retryBody})
	if err == nil || messaging.IsPermanent(err) {
		t.Fatalf("transient identity failure error = %v, want retryable", err)
	}
	if got := mail.recipients()[customerEmail]; got != 1 {
		t.Errorf("emails after transient failure = %d, want 1", got)
	}
	identityFail.Store(false)
	if err := notification.Handle(ctx, messaging.Message{Body: retryBody}); err != nil {
		t.Fatalf("retry after identity recovery: %v", err)
	}
	if got := mail.recipients()[customerEmail]; got != 2 {
		t.Errorf("emails after retry = %d, want 2", got)
	}

	// A failed webhook delivery records the attempt and schedules attempt 2.
	sender.failNext.Store(true)
	webhookEnvelope := syntheticMerchantEnvelope(t, merchantID)
	webhookBody, _ := json.Marshal(webhookEnvelope)
	if err := webhook.Handle(ctx, messaging.Message{Body: webhookBody}); err != nil {
		t.Fatalf("failed webhook attempt: %v", err)
	}
	if scheduled := scheduler.calls(); len(scheduled) != 1 || scheduled[0].nextAttempt != 2 {
		t.Fatalf("scheduler calls = %+v, want attempt 2", scheduled)
	}
	sender.failNext.Store(false)
	if err := webhook.Handle(ctx, messaging.Message{Body: webhookBody, Headers: map[string]any{"x-webhook-attempt": int32(2)}}); err != nil {
		t.Fatalf("webhook retry: %v", err)
	}
	calls := sender.calls()
	if len(calls) != 3 {
		t.Fatalf("webhook sender calls = %d, want 3", len(calls))
	}
	if calls[1].event.ID != webhookEnvelope.ID || calls[2].event.ID != webhookEnvelope.ID {
		t.Errorf("webhook retry event IDs = %s/%s, want %s", calls[1].event.ID, calls[2].event.ID, webhookEnvelope.ID)
	}

	// Malformed events are permanent, never retried forever.
	if err := realtime.Handle(ctx, messaging.Message{Body: []byte(`not-json`)}); err == nil || !messaging.IsPermanent(err) {
		t.Errorf("invalid realtime event error = %v, want permanent", err)
	}
}

// --- fixtures and recording collaborators ---

func insertMerchant(t *testing.T, pool *postgres.Pool, merchantID, contactEmail string) {
	t.Helper()
	ownerID := newUUID(t, pool)
	zoneID := newUUID(t, pool)
	_, err := pool.Exec(t.Context(), `INSERT INTO merchant.merchants (
		id, owner_user_id, business_name, business_reg_no, contact_email, contact_phone,
		pickup_address, pickup_zone_id, status, approved_by, approved_at
	) VALUES ($1,$2,$3,$4,$5,'+94771234567','12 Galle Road',$6,'APPROVED',$2,now())`,
		merchantID, ownerID, "Event Flow Merchant", "EF-"+merchantID[:8], contactEmail, zoneID)
	if err != nil {
		t.Fatalf("insert merchant: %v", err)
	}
}

func insertMerchantOrder(t *testing.T, pool *postgres.Pool, merchantID, parcelID, tracking string) {
	t.Helper()
	_, err := pool.Exec(t.Context(), `INSERT INTO merchant.merchant_orders (
		merchant_id, external_order_id, request_hash, parcel_id, tracking_number, recipient_name,
		dropoff_city, weight_kg, size_category, fee_amount, fee_breakdown, last_known_status, last_status_at
	) VALUES ($1,$2,$3,$4,$5,'Recipient','Kandy',2,'SMALL',550,
		'{"base_fee":450,"weight_charge":100,"size_surcharge":0}','CREATED',now() - interval '1 minute')`,
		merchantID, "EVENTFLOW-"+parcelID[:8], randomHex(32), parcelID, tracking)
	if err != nil {
		t.Fatalf("insert merchant order: %v", err)
	}
}

func insertParcel(t *testing.T, pool *postgres.Pool, ownerID string, merchant bool) (string, string) {
	t.Helper()
	id := newUUID(t, pool)
	courierID := newUUID(t, pool)
	tracking := "SDK" + strings.ToUpper(strings.ReplaceAll(id, "-", "")[:8])
	var customer, merchantID any = ownerID, nil
	if merchant {
		customer, merchantID = nil, ownerID
		tracking = "SDN" + strings.ToUpper(strings.ReplaceAll(id, "-", "")[:8])
	}
	_, err := pool.Exec(t.Context(), `INSERT INTO parcel.parcels (
		id, tracking_number, customer_user_id, merchant_id, sender_name, sender_phone,
		pickup_address, pickup_city, pickup_zone_id, recipient_name, recipient_phone,
		dropoff_address, dropoff_city, dropoff_zone_id, weight_kg, size_category,
		status, current_courier_id, price_amount, price_breakdown, created_by_user_id
	) VALUES ($1,$2,$3,$4,'Sender','+94770000001','Pickup','Colombo',$5,
		'Recipient','+94770000002','Dropoff','Kandy',$5,2,'SMALL','CREATED',$6,550,
		'{"base_fee":450,"weight_charge":100,"size_surcharge":0}',$5)`,
		id, tracking, customer, merchantID, courierID, courierID)
	if err != nil {
		t.Fatalf("insert parcel: %v", err)
	}
	if _, err := pool.Exec(t.Context(), `INSERT INTO parcel.parcel_assignments (parcel_id,courier_id,assigned_by) VALUES ($1,$2,$2)`, id, courierID); err != nil {
		t.Fatalf("insert assignment: %v", err)
	}
	return id, tracking
}

func courierIDFor(t *testing.T, pool *postgres.Pool, parcelID string) string {
	t.Helper()
	var courierID string
	if err := pool.QueryRow(t.Context(), `SELECT courier_id::text FROM parcel.parcel_assignments WHERE parcel_id=$1`, parcelID).Scan(&courierID); err != nil {
		t.Fatalf("read courier assignment: %v", err)
	}
	return courierID
}

func newUUID(t *testing.T, pool *postgres.Pool) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(t.Context(), `SELECT gen_random_uuid()::text`).Scan(&id); err != nil {
		t.Fatalf("generate UUID: %v", err)
	}
	return id
}

func syntheticCustomerEnvelope(t *testing.T, customerID string) messaging.Envelope {
	t.Helper()
	return syntheticEnvelope(t, customerID, "", "SDKRETRY001")
}

func syntheticMerchantEnvelope(t *testing.T, merchantID string) messaging.Envelope {
	t.Helper()
	return syntheticEnvelope(t, "", merchantID, "SDNRETRY001")
}

func syntheticEnvelope(t *testing.T, customerID, merchantID, tracking string) messaging.Envelope {
	t.Helper()
	data := map[string]any{
		"parcel_id":       randomUUID(),
		"tracking_number": tracking,
		"previous_status": "CREATED",
		"status":          "PICKED_UP",
	}
	if customerID != "" {
		data["customer_user_id"] = customerID
	}
	if merchantID != "" {
		data["merchant_id"] = merchantID
	}
	encoded, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("marshal event data: %v", err)
	}
	return messaging.Envelope{
		ID: randomUUID(), Type: eventType, Version: 1,
		OccurredAt: time.Now().UTC(), Producer: "parcel", Data: encoded,
	}
}

func outboxEventIDs(t *testing.T, pool *postgres.Pool) map[string]string {
	t.Helper()
	rows, err := pool.Query(t.Context(), `SELECT id::text FROM parcel.outbox_events WHERE event_type='parcel.status_changed'`)
	if err != nil {
		t.Fatalf("query outbox: %v", err)
	}
	defer rows.Close()
	ids := map[string]string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("scan outbox id: %v", err)
		}
		ids[id] = id
	}
	return ids
}

func classifyEnvelopes(t *testing.T, envelopes []messaging.Envelope, customerID, merchantID string) (messaging.Envelope, messaging.Envelope) {
	t.Helper()
	var customer, merchant messaging.Envelope
	for _, envelope := range envelopes {
		var data map[string]any
		if err := json.Unmarshal(envelope.Data, &data); err != nil {
			t.Fatalf("decode envelope data: %v", err)
		}
		switch data["customer_user_id"] {
		case customerID:
			customer = envelope
		case nil:
			if data["merchant_id"] == merchantID {
				merchant = envelope
			}
		}
	}
	if customer.ID == "" || merchant.ID == "" {
		t.Fatalf("did not classify both envelopes: customer=%s merchant=%s", customer.ID, merchant.ID)
	}
	return customer, merchant
}

func scalar(t *testing.T, pool *postgres.Pool, statement string) string {
	t.Helper()
	var value string
	if err := pool.QueryRow(t.Context(), statement).Scan(&value); err != nil {
		t.Fatalf("query %q: %v", statement, err)
	}
	return value
}

// --- recording collaborators ---

type captureMailer struct {
	mu       sync.Mutex
	messages []mailer.Message
}

func (m *captureMailer) Send(_ context.Context, message mailer.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = append(m.messages, message)
	return nil
}

func (m *captureMailer) snapshot() []mailer.Message {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]mailer.Message(nil), m.messages...)
}

func (m *captureMailer) recipients() map[string]int {
	counts := map[string]int{}
	for _, message := range m.snapshot() {
		counts[message.To]++
	}
	return counts
}

type capturedPublish struct {
	exchange string
	key      string
	body     []byte
}

type recordingPublisher struct {
	inner parcelevents.Publisher
	mu    sync.Mutex
	items []capturedPublish
}

func (p *recordingPublisher) Publish(ctx context.Context, exchange, key string, body []byte) error {
	if err := p.inner.Publish(ctx, exchange, key, body); err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.items = append(p.items, capturedPublish{exchange: exchange, key: key, body: append([]byte(nil), body...)})
	return nil
}

func (p *recordingPublisher) snapshot() []capturedPublish {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]capturedPublish(nil), p.items...)
}

func (p *recordingPublisher) envelopes(t *testing.T) []messaging.Envelope {
	t.Helper()
	items := p.snapshot()
	envelopes := make([]messaging.Envelope, 0, len(items))
	for _, item := range items {
		if item.key != routingKey {
			t.Errorf("published routing key = %q, want %q", item.key, routingKey)
		}
		var envelope messaging.Envelope
		if err := json.Unmarshal(item.body, &envelope); err != nil {
			t.Fatalf("decode published envelope: %v", err)
		}
		if err := envelope.Validate(); err != nil {
			t.Fatalf("published envelope is invalid: %v", err)
		}
		envelopes = append(envelopes, envelope)
	}
	return envelopes
}

type deliveryCall struct {
	target webhookdelivery.Target
	event  webhookdelivery.Event
}

type recordingSender struct {
	failNext atomic.Bool
	mu       sync.Mutex
	items    []deliveryCall
}

func (s *recordingSender) Send(_ context.Context, target webhookdelivery.Target, event webhookdelivery.Event) webhookdelivery.Result {
	s.mu.Lock()
	s.items = append(s.items, deliveryCall{target: target, event: event})
	s.mu.Unlock()
	if s.failNext.Swap(false) {
		return webhookdelivery.Result{StatusCode: http.StatusServiceUnavailable, ErrorMessage: "upstream unavailable"}
	}
	return webhookdelivery.Result{StatusCode: http.StatusOK, Success: true}
}

func (s *recordingSender) calls() []deliveryCall {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]deliveryCall(nil), s.items...)
}

type scheduledCall struct {
	nextAttempt int
	delay       time.Duration
}

type recordingScheduler struct {
	mu    sync.Mutex
	items []scheduledCall
}

func (s *recordingScheduler) Schedule(_ context.Context, _ messaging.Message, nextAttempt int, delay time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items = append(s.items, scheduledCall{nextAttempt: nextAttempt, delay: delay})
	return nil
}

func (s *recordingScheduler) calls() []scheduledCall {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]scheduledCall(nil), s.items...)
}

type recordingBroadcaster struct {
	mu    sync.Mutex
	items []realtimeevents.Update
}

func (b *recordingBroadcaster) Broadcast(update realtimeevents.Update) int {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.items = append(b.items, update)
	return 1
}

func (b *recordingBroadcaster) updates() []realtimeevents.Update {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]realtimeevents.Update(nil), b.items...)
}

type fakeMerchantAPI struct {
	mu       sync.Mutex
	recorded []webhookevents.Attempt
}

func newFakeMerchantAPI() *fakeMerchantAPI { return &fakeMerchantAPI{} }

func (f *fakeMerchantAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	switch {
	case strings.HasSuffix(r.URL.Path, "/webhook"):
		_, _ = w.Write([]byte(`{"data":{"url":"https://merchant.example.test/hook","signingSecret":"whsec_eventflow","eventTypes":["parcel.status_changed"],"isActive":true,"merchantStatus":"APPROVED"}}`))
	case strings.HasSuffix(r.URL.Path, "/webhook/attempts"):
		var attempt webhookevents.Attempt
		if err := json.NewDecoder(r.Body).Decode(&attempt); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		f.mu.Lock()
		f.recorded = append(f.recorded, attempt)
		f.mu.Unlock()
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"data":{"eventId":"` + attempt.EventID + `","attemptNumber":1,"recorded":true}}`))
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func (f *fakeMerchantAPI) attempts() []webhookevents.Attempt {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]webhookevents.Attempt(nil), f.recorded...)
}

func startConsumer(t *testing.T, ctx context.Context, rabbitURL, exchange, queue string, handler messaging.Handler, logger *slog.Logger) {
	t.Helper()
	config := messaging.ConsumerConfig{
		URL: rabbitURL, Exchange: exchange, Queue: queue, RoutingKey: routingKey,
		Prefetch: 1, HandlerTimeout: 10 * time.Second, MaxRetries: 1, RetryDelay: time.Second,
		ReconnectMin: 100 * time.Millisecond, ReconnectMax: time.Second,
	}
	go func() {
		if err := messaging.Run(ctx, config, handler, logger); err != nil && ctx.Err() == nil {
			t.Logf("consumer %s stopped: %v", queue, err)
		}
	}()
}

func waitForConsumer(t *testing.T, rabbitURL, queue string) {
	t.Helper()
	waitUntil(t, eventWaitTimeout, "consumer on "+queue, func() error {
		connection, err := amqp.Dial(rabbitURL)
		if err != nil {
			return err
		}
		defer connection.Close()
		channel, err := connection.Channel()
		if err != nil {
			return err
		}
		defer channel.Close()
		info, err := channel.QueueInspect(queue)
		if err != nil {
			return err
		}
		if info.Consumers < 1 {
			return fmt.Errorf("queue %s has no consumers", queue)
		}
		return nil
	})
}

func deleteTopology(t *testing.T, rabbitURL, exchange string, queues ...string) {
	t.Helper()
	connection, err := amqp.Dial(rabbitURL)
	if err != nil {
		t.Logf("cleanup dial: %v", err)
		return
	}
	defer connection.Close()
	channel, err := connection.Channel()
	if err != nil {
		t.Logf("cleanup channel: %v", err)
		return
	}
	defer channel.Close()
	for _, queue := range queues {
		for _, name := range []string{queue, queue + ".retry.v2", queue + ".dlq"} {
			if _, err := channel.QueueDelete(name, false, false, false); err != nil {
				t.Logf("delete queue %s: %v", name, err)
			}
		}
	}
	for _, name := range []string{exchange, exchange + ".retry", exchange + ".dlx"} {
		if err := channel.ExchangeDelete(name, false, false); err != nil {
			t.Logf("delete exchange %s: %v", name, err)
		}
	}
}

func waitUntil(t *testing.T, timeout time.Duration, description string, check func() error) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var last error
	for time.Now().Before(deadline) {
		if err := check(); err == nil {
			return
		} else {
			last = err
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s: %v", description, last)
}
