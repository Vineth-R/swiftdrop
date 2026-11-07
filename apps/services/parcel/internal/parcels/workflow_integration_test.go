//go:build integration && postgres

package parcels

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Vineth-R/swiftdrop/packages/go-common/postgres"
	"github.com/Vineth-R/swiftdrop/tests/integration/parceltest"
)

// Pricing is the only isolated dependency in these tests. The real application
// service, repository, PostgreSQL constraints, migrations and transactions run.
type workflowQuotes struct {
	total            atomic.Int64
	calls            atomic.Int64
	unavailable      atomic.Bool
	changeAfterFirst atomic.Bool
}

func newWorkflowQuotes() *workflowQuotes {
	quotes := &workflowQuotes{}
	quotes.total.Store(55000)
	return quotes
}

func (q *workflowQuotes) Quote(context.Context, CreateInput) (Fee, error) {
	calls := q.calls.Add(1)
	if q.unavailable.Load() {
		return Fee{}, ErrUnavailable
	}
	total := Decimal(q.total.Load())
	if q.changeAfterFirst.Load() && calls == 1 {
		q.total.Store(60000)
	}
	return Fee{Currency: "LKR", BaseFee: total - 10000, WeightCharge: 10000, Total: total}, nil
}

func workflowInput(t *testing.T) CreateInput {
	t.Helper()
	expected := Decimal(55000)
	description := "Books"
	return CreateInput{
		Sender:    Contact{Name: "Nimal Perera", Phone: "+94771234567", Address: "12 Temple Road", City: "Colombo", ZoneID: parceltest.UUID(t)},
		Recipient: Contact{Name: "Amali Fernando", Phone: "+94712345678", Address: "45 Lake Drive", City: "Kandy", ZoneID: parceltest.UUID(t)},
		WeightKG:  200, SizeCategory: "SMALL", Description: &description, ExpectedFee: &expected,
	}
}

func workflowService(database *parceltest.Database, quotes *workflowQuotes) *Service {
	return NewService(NewStore(database.Pool), quotes)
}

func TestParcelWorkflowAtomicCreationAndEvent(t *testing.T) {
	database := parceltest.New(t)
	quotes := newWorkflowQuotes()
	service := workflowService(database, quotes)
	customer := parceltest.UUID(t)
	input := workflowInput(t)
	// PostgreSQL canonicalizes UUID text; an identical valid uppercase request
	// must still replay the same parcel after that normalization.
	input.Sender.ZoneID = strings.ToUpper(input.Sender.ZoneID)
	input.Recipient.ZoneID = strings.ToUpper(input.Recipient.ZoneID)
	key := parceltest.UUID(t)
	created, err := service.CreateCustomer(t.Context(), customer, key, input)
	if err != nil {
		t.Fatalf("create customer parcel: %v", err)
	}
	if !regexp.MustCompile(`^SDK[A-Z0-9]{8}$`).MatchString(created.TrackingNumber) || created.Status != "CREATED" {
		t.Fatalf("invalid tracking number or initial status: %+v", created)
	}
	if created.Fee.Total != 55000 || created.Fee.BaseFee != 45000 || created.Fee.WeightCharge != 10000 || created.Fee.Currency != "LKR" {
		t.Fatalf("accepted fee did not round-trip exactly: %+v", created.Fee)
	}
	replayed, err := service.CreateCustomer(t.Context(), customer, key, input)
	if err != nil || replayed.ID != created.ID {
		t.Fatalf("same uppercase UUID request did not replay: %+v %v", replayed, err)
	}
	history, err := service.History(t.Context(), created.ID, customer, false)
	if err != nil || len(history) != 1 || history[0].Status != "CREATED" || history[0].PreviousStatus != nil {
		t.Fatalf("initial history = %+v; error = %v", history, err)
	}
	if history[0].ActorRole == nil || *history[0].ActorRole != "CUSTOMER" {
		t.Fatalf("initial actor role = %v", history[0].ActorRole)
	}
	var eventType, routing, aggregateType, aggregateID string
	var payload []byte
	if err := database.Pool.QueryRow(t.Context(), `SELECT event_type, routing_key, aggregate_type, aggregate_id::text, payload
		FROM parcel.outbox_events WHERE aggregate_id = $1`, created.ID).Scan(&eventType, &routing, &aggregateType, &aggregateID, &payload); err != nil {
		t.Fatalf("read creation outbox event: %v", err)
	}
	if eventType != "parcel.created" || routing != "parcel.created" || aggregateType != "parcel" || aggregateID != created.ID {
		t.Fatalf("wrong storage-to-bus contract: %s %s %s %s", eventType, routing, aggregateType, aggregateID)
	}
	var data map[string]any
	if err := json.Unmarshal(payload, &data); err != nil {
		t.Fatal(err)
	}
	if len(data) != 4 || data["parcel_id"] != created.ID || data["tracking_number"] != created.TrackingNumber || data["status"] != "CREATED" || data["customer_user_id"] != customer {
		t.Fatalf("created event must contain only schema fields and customer owner: %s", payload)
	}
	for _, private := range []string{input.Sender.Name, input.Sender.Phone, input.Sender.Address, input.Recipient.Name, input.Recipient.Phone, input.Recipient.Address} {
		if strings.Contains(string(payload), private) {
			t.Fatal("created event leaks contact details")
		}
	}
}

func TestParcelWorkflowConcurrentIdempotencyAndOwnership(t *testing.T) {
	database := parceltest.New(t)
	quotes := newWorkflowQuotes()
	quotes.changeAfterFirst.Store(true)
	service := workflowService(database, quotes)
	customer, key := parceltest.UUID(t), parceltest.UUID(t)
	input := workflowInput(t)
	type result struct {
		parcel Detail
		err    error
	}
	const requests = 8
	start := make(chan struct{})
	results := make(chan result, requests)
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	for range requests {
		go func() {
			<-start
			parcel, err := service.CreateCustomer(ctx, customer, key, input)
			results <- result{parcel, err}
		}()
	}
	close(start)
	var winner Detail
	for range requests {
		got := <-results
		if got.err != nil {
			t.Errorf("concurrent request: %v", got.err)
			continue
		}
		if winner.ID == "" {
			winner = got.parcel
		}
		if winner.ID != got.parcel.ID || winner.TrackingNumber != got.parcel.TrackingNumber {
			t.Error("same intended submission created multiple parcels")
		}
	}
	if winner.ID == "" {
		t.Fatal("no concurrent submission succeeded")
	}
	if quotes.calls.Load() != 1 {
		t.Fatalf("concurrent idempotent requests quoted %d times, want one accepted snapshot", quotes.calls.Load())
	}
	var count int
	if err := database.Pool.QueryRow(t.Context(), `SELECT count(*) FROM parcel.parcels WHERE customer_user_id = $1 AND idempotency_key = $2`, customer, key).Scan(&count); err != nil || count != 1 {
		t.Fatalf("persisted duplicates: count=%d error=%v", count, err)
	}
	for _, table := range []string{"parcel_status_history", "outbox_events"} {
		column := "parcel_id"
		if table == "outbox_events" {
			column = "aggregate_id"
		}
		// The table and column names are fixed test literals, never user input.
		if err := database.Pool.QueryRow(t.Context(), "SELECT count(*) FROM parcel."+table+" WHERE "+column+" = $1", winner.ID).Scan(&count); err != nil || count != 1 {
			t.Fatalf("duplicate creation side effects in %s: count=%d error=%v", table, count, err)
		}
	}
	quotes.unavailable.Store(true)
	replayed, err := service.CreateCustomer(t.Context(), customer, key, input)
	if err != nil || replayed.ID != winner.ID {
		t.Fatalf("persisted replay must not need current pricing: %+v %v", replayed, err)
	}
	quotes.unavailable.Store(false)
	changed := input
	changed.Recipient.Name = "Changed recipient"
	if _, err := service.CreateCustomer(t.Context(), customer, key, changed); !errors.Is(err, ErrConflict) {
		t.Fatalf("same key with changed payload = %v, want conflict", err)
	}
	other := parceltest.UUID(t)
	if _, err := service.Detail(t.Context(), winner.ID, other, false); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-customer detail = %v, want not found", err)
	}
	if _, err := service.History(t.Context(), winner.ID, other, false); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-customer history = %v, want not found", err)
	}
	quotes.total.Store(55000)
	otherCreated, err := service.CreateCustomer(t.Context(), other, key, input)
	if err != nil || otherCreated.ID == winner.ID {
		t.Fatalf("idempotency must be customer-scoped: %+v %v", otherCreated, err)
	}
	if _, err := service.Detail(t.Context(), parceltest.UUID(t), customer, false); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing detail = %v, want not found", err)
	}
}

func TestParcelWorkflowRollbackWhenOutboxRejects(t *testing.T) {
	database := parceltest.New(t)
	service := workflowService(database, newWorkflowQuotes())
	// This fixture owns a generated database. The constraint injects a failure
	// at the final persistence step without a production-only test seam.
	if _, err := database.Pool.Exec(t.Context(), `ALTER TABLE parcel.outbox_events ADD CONSTRAINT test_reject_creation CHECK (false) NOT VALID`); err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreateCustomer(t.Context(), parceltest.UUID(t), parceltest.UUID(t), workflowInput(t)); err == nil {
		t.Fatal("creation succeeded despite outbox rejection")
	}
	for _, table := range []string{"parcels", "parcel_status_history", "outbox_events"} {
		var count int
		// Table names are fixed test literals.
		if err := database.Pool.QueryRow(t.Context(), "SELECT count(*) FROM parcel."+table).Scan(&count); err != nil || count != 0 {
			t.Fatalf("rollback left records in %s: count=%d error=%v", table, count, err)
		}
	}
}

func TestParcelWorkflowStoredPriceAndApplicationRestart(t *testing.T) {
	database := parceltest.New(t)
	quotes := newWorkflowQuotes()
	service := workflowService(database, quotes)
	customer := parceltest.UUID(t)
	input := workflowInput(t)
	key := parceltest.UUID(t)
	created, err := service.CreateCustomer(t.Context(), customer, key, input)
	if err != nil {
		t.Fatal(err)
	}
	quotes.total.Store(60000)
	_, err = service.CreateCustomer(t.Context(), customer, parceltest.UUID(t), input)
	var changed *PriceChanged
	if !errors.As(err, &changed) || changed.CurrentFee.Total != 60000 {
		t.Fatalf("changed pricing = %v, want current fee 600.00", err)
	}
	var count int
	if err := database.Pool.QueryRow(t.Context(), `SELECT count(*) FROM parcel.parcels`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("price mismatch persisted a parcel: count=%d error=%v", count, err)
	}
	database.Pool.Close()
	config := postgres.DefaultConfig()
	config.URL, config.MinConns, config.RetryAttempts = database.URL(), 0, 1
	database.Pool, err = postgres.Open(t.Context(), config)
	if err != nil {
		t.Fatal("reopen application database pool")
	}
	// A newly constructed application with a fresh pool proves persistence is
	// in PostgreSQL, rather than process memory or an idempotency cache.
	restarted := workflowService(database, quotes)
	stored, err := restarted.Detail(t.Context(), created.ID, customer, false)
	if err != nil || stored.Fee != created.Fee || stored.TrackingNumber != created.TrackingNumber {
		t.Fatalf("restart changed stored parcel/price: %+v %v", stored, err)
	}
	if quotes.calls.Load() != 2 {
		t.Fatalf("read recalculated stored price: quote calls=%d, want 2", quotes.calls.Load())
	}
	replayed, err := restarted.CreateCustomer(t.Context(), customer, key, input)
	if err != nil || replayed.ID != created.ID || replayed.Fee != created.Fee {
		t.Fatalf("idempotency did not survive application restart: %+v %v", replayed, err)
	}
	rawReceipt, err := Receipt(stored)
	if err != nil {
		t.Fatal(err)
	}
	receipt := string(rawReceipt)
	for _, text := range []string{"%PDF-1.4", stored.TrackingNumber, stored.Sender.Name, stored.Recipient.Name, "Base fee: 450.00", "Weight charge: 100.00", "Size surcharge: 0.00", "Total fee: 550.00", "Currency: LKR", "%%EOF"} {
		if !strings.Contains(receipt, text) {
			t.Fatalf("receipt omitted stored parcel information %q", text)
		}
	}
	if strings.Contains(receipt, "Total fee: 600.00") {
		t.Fatal("receipt used changed pricing instead of stored fee")
	}
}

func TestParcelWorkflowListHistoryMerchantAndSummary(t *testing.T) {
	database := parceltest.New(t)
	service := workflowService(database, newWorkflowQuotes())
	customer := parceltest.UUID(t)
	input := workflowInput(t)
	seen := map[string]bool{}
	var first Detail
	for range 5 {
		created, err := service.CreateCustomer(t.Context(), customer, parceltest.UUID(t), input)
		if err != nil {
			t.Fatal(err)
		}
		if seen[created.TrackingNumber] {
			t.Fatal("duplicate tracking number")
		}
		seen[created.TrackingNumber] = true
		if first.ID == "" {
			first = created
		}
	}
	list, total, err := service.List(t.Context(), customer, Filter{Status: "CREATED", Page: 2, PageSize: 2, Sort: "createdAt:asc"})
	if err != nil || len(list) != 2 || total != 5 {
		t.Fatalf("list page=%+v total=%d error=%v", list, total, err)
	}
	if list[0].CreatedAt.After(list[1].CreatedAt) {
		t.Fatal("list sort order is wrong")
	}
	beyond, total, err := service.List(t.Context(), customer, Filter{Page: 10, PageSize: 2, Sort: "createdAt:desc"})
	if err != nil || len(beyond) != 0 || total != 5 {
		t.Fatalf("page beyond end lost accurate total: %+v %d %v", beyond, total, err)
	}
	tomorrow := time.Now().UTC().AddDate(0, 0, 1)
	beyond, total, err = service.List(t.Context(), customer, Filter{From: &tomorrow, Page: 1, PageSize: 20, Sort: "createdAt:desc"})
	if err != nil || len(beyond) != 0 || total != 0 {
		t.Fatalf("list ignored creation date filter: %+v %d %v", beyond, total, err)
	}
	empty, total, err := service.List(t.Context(), parceltest.UUID(t), Filter{Page: 1, PageSize: 20, Sort: "createdAt:desc"})
	if err != nil || len(empty) != 0 || total != 0 {
		t.Fatalf("list leaked another customer's parcels: %+v %d %v", empty, total, err)
	}
	empty, total, err = service.List(t.Context(), customer, Filter{Status: "DELIVERED", Page: 1, PageSize: 20, Sort: "createdAt:desc"})
	if err != nil || len(empty) != 0 || total != 0 {
		t.Fatalf("list ignored status filter: %+v %d %v", empty, total, err)
	}
	// Append-only history remains the source of the timeline; use deliberately
	// different timestamps to prove chronological reads independently of IDs.
	for _, step := range []struct{ previous, next string }{{"CREATED", "PICKED_UP"}, {"PICKED_UP", "IN_TRANSIT"}} {
		if _, err := database.Pool.Exec(t.Context(), `UPDATE parcel.parcels SET status=$2 WHERE id=$1`, first.ID, step.next); err != nil {
			t.Fatal(err)
		}
		if _, err := database.Pool.Exec(t.Context(), `INSERT INTO parcel.parcel_status_history (parcel_id, previous_status, new_status, actor_role, created_at)
			VALUES ($1,$2,$3::varchar,'COURIER',now()+CASE WHEN $3::varchar='PICKED_UP' THEN interval '1 second' ELSE interval '2 seconds' END)`, first.ID, step.previous, step.next); err != nil {
			t.Fatal(err)
		}
	}
	history, err := service.History(t.Context(), first.ID, customer, false)
	if err != nil || len(history) != 3 || history[0].Status != "CREATED" || history[1].Status != "PICKED_UP" || history[2].Status != "IN_TRANSIT" {
		t.Fatalf("chronological history=%+v error=%v", history, err)
	}
	merchantID := parceltest.UUID(t)
	merchant := MerchantInput{MerchantID: merchantID, CreatedByUserID: parceltest.UUID(t), ExternalOrderID: "SHOP-100", IdempotencyKey: "SHOP-100", Pickup: input.Sender, Recipient: input.Recipient, WeightKG: input.WeightKG, SizeCategory: input.SizeCategory, Description: input.Description}
	created, err := service.CreateMerchant(t.Context(), merchant)
	if err != nil || !regexp.MustCompile(`^SDN[A-Z0-9]{8}$`).MatchString(created.TrackingNumber) {
		t.Fatalf("merchant parcel=%+v error=%v", created, err)
	}
	replay, err := service.CreateMerchant(t.Context(), merchant)
	if err != nil || replay.ID != created.ID {
		t.Fatalf("merchant external-order replay=%+v error=%v", replay, err)
	}
	summary, err := service.Summary(t.Context(), created.ID)
	if err != nil || summary.ParcelID != created.ID || summary.CustomerUserID != nil || summary.MerchantID == nil || *summary.MerchantID != merchantID || summary.ExternalOrderID == nil || *summary.ExternalOrderID != "SHOP-100" || summary.RecipientName != input.Recipient.Name {
		t.Fatalf("merchant summary=%+v error=%v", summary, err)
	}
	if _, err := service.Detail(t.Context(), created.ID, customer, false); !errors.Is(err, ErrNotFound) {
		t.Fatalf("customer accessed merchant parcel: %v", err)
	}
	merchant.Recipient.Name = "Different payload"
	if _, err := service.CreateMerchant(t.Context(), merchant); !errors.Is(err, ErrConflict) {
		t.Fatalf("merchant conflicting replay=%v", err)
	}
	if _, err := service.Summary(t.Context(), parceltest.UUID(t)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing internal summary=%v", err)
	}
}
