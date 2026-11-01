//go:build integration && postgres

package repository

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/Vineth-R/swiftdrop/tests/integration/parceltest"
)

func TestParcelRepositoryRoundTrip(t *testing.T) {
	database := parceltest.New(t)
	repository := testRepository(t, database.Pool)
	ctx := context.Background()
	params := testParcelParams(t)
	params.SenderName = "O'Neil"
	description := "Books'); SELECT 'still data"
	params.Description = &description
	key := "customer-request"
	params.IdempotencyKey = &key
	created, err := repository.CreateParcel(ctx, params)
	if err != nil {
		t.Fatalf("CreateParcel(): %v", err)
	}
	if created.Status != ParcelStatusCreated || created.CreatedAt.IsZero() || created.UpdatedAt.IsZero() {
		t.Fatal("created parcel is missing its initial status or database timestamps")
	}
	lookups := []struct {
		name string
		run  func() (Parcel, error)
	}{
		{name: "id", run: func() (Parcel, error) { return repository.ParcelByID(ctx, created.ID) }},
		{name: "customer key", run: func() (Parcel, error) {
			return repository.ParcelByCustomerIdempotencyKey(ctx, *params.CustomerUserID, key)
		}},
	}
	for _, lookup := range lookups {
		t.Run(lookup.name, func(t *testing.T) {
			parcel, err := lookup.run()
			if err != nil {
				t.Fatalf("lookup: %v", err)
			}
			if parcel.ID != params.ID || parcel.TrackingNumber != params.TrackingNumber {
				t.Fatal("lookup returned a different parcel")
			}
			if parcel.SenderName != params.SenderName || parcel.Description == nil || *parcel.Description != description {
				t.Fatal("text values did not round-trip unchanged")
			}
			if parcel.CustomerUserID == nil || *parcel.CustomerUserID != *params.CustomerUserID {
				t.Fatal("customer owner did not round-trip")
			}
			if parcel.MerchantID != nil || parcel.DeliveredAt != nil || parcel.FailedAt != nil {
				t.Fatal("nullable fields were not preserved")
			}
			if parcel.FailureReason != nil || parcel.DeliveredToName != nil || parcel.CurrentCourierID != nil {
				t.Fatal("new parcel unexpectedly has delivery or assignment metadata")
			}
			amount, err := parcel.PriceAmount.Value()
			if err != nil || amount != "550.25" {
				t.Fatalf("exact amount = %v, error = %v, want 550.25", amount, err)
			}
			weight, err := parcel.WeightKG.Value()
			if err != nil || weight != "2.10" {
				t.Fatalf("exact weight = %v, error = %v, want 2.10", weight, err)
			}
			var breakdown map[string]json.Number
			if err := json.Unmarshal(parcel.PriceBreakdown, &breakdown); err != nil {
				t.Fatalf("decode stored breakdown: %v", err)
			}
			if breakdown["base_fee"].String() != "450.25" {
				t.Fatal("price breakdown did not preserve its decimal value")
			}
		})
	}
	if _, err := repository.ParcelByCustomerIdempotencyKey(ctx, parceltest.UUID(t), key); !errors.Is(err, ErrNotFound) {
		t.Fatalf("another customer's lookup = %v, want ErrNotFound", err)
	}
	otherCustomer := testParcelParams(t)
	otherCustomer.IdempotencyKey = &key
	if _, err := repository.CreateParcel(ctx, otherCustomer); err != nil {
		t.Fatalf("another customer should be allowed the same key: %v", err)
	}
}

func TestParcelRepositoryDuplicateErrors(t *testing.T) {
	database := parceltest.New(t)
	repository := testRepository(t, database.Pool)
	ctx := context.Background()
	tests := []struct {
		name   string
		change func(*CreateParcelParams)
		want   error
	}{
		{name: "tracking", change: func(params *CreateParcelParams) {
			params.ID = parceltest.UUID(t)
		}, want: ErrDuplicateTrackingNumber},
		{name: "customer idempotency", change: func(params *CreateParcelParams) {
			params.ID = parceltest.UUID(t)
			params.TrackingNumber = testParcelParams(t).TrackingNumber
		}, want: ErrDuplicateIdempotencyKey},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			params := testParcelParams(t)
			key := parceltest.UUID(t)
			params.IdempotencyKey = &key
			if _, err := repository.CreateParcel(ctx, params); err != nil {
				t.Fatalf("initial creation: %v", err)
			}
			test.change(&params)
			if _, err := repository.CreateParcel(ctx, params); !errors.Is(err, test.want) {
				t.Fatalf("duplicate creation = %v, want %v", err, test.want)
			}
		})
	}
	merchant := testParcelParams(t)
	merchant.MerchantID = merchant.CustomerUserID
	merchant.CustomerUserID = nil
	merchant.TrackingNumber = strings.Replace(merchant.TrackingNumber, "SDK", "SDN", 1)
	externalOrder := "external-order"
	merchant.ExternalOrderID = &externalOrder
	created, err := repository.CreateParcel(ctx, merchant)
	if err != nil {
		t.Fatalf("merchant creation: %v", err)
	}
	found, err := repository.ParcelByMerchantExternalOrderID(ctx, *merchant.MerchantID, externalOrder)
	if err != nil || found.ID != created.ID {
		t.Fatalf("merchant order lookup: %v", err)
	}
	_, err = repository.ParcelByMerchantExternalOrderID(ctx, parceltest.UUID(t), externalOrder)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("another merchant's lookup = %v, want ErrNotFound", err)
	}
	merchant.ID = parceltest.UUID(t)
	merchant.TrackingNumber = "SDN" + strings.ToUpper(strings.ReplaceAll(merchant.ID, "-", "")[:8])
	if _, err := repository.CreateParcel(ctx, merchant); !errors.Is(err, ErrDuplicateMerchantOrder) {
		t.Fatalf("merchant duplicate = %v, want ErrDuplicateMerchantOrder", err)
	}
}

func TestParcelRepositoryConcurrentIdempotency(t *testing.T) {
	database := parceltest.New(t)
	repository := testRepository(t, database.Pool)
	first := testParcelParams(t)
	key := "concurrent-request"
	first.IdempotencyKey = &key
	second := first
	second.ID = parceltest.UUID(t)
	second.TrackingNumber = "SDK" + strings.ToUpper(strings.ReplaceAll(second.ID, "-", "")[:8])
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	start := make(chan struct{})
	results := make(chan error, 2)
	for _, params := range []CreateParcelParams{first, second} {
		go func() {
			<-start
			_, err := repository.CreateParcel(ctx, params)
			results <- err
		}()
	}
	close(start)
	created, duplicates := 0, 0
	for range 2 {
		err := <-results
		switch {
		case err == nil:
			created++
		case errors.Is(err, ErrDuplicateIdempotencyKey):
			duplicates++
		default:
			t.Errorf("concurrent creation: %v", err)
		}
	}
	if created != 1 || duplicates != 1 {
		t.Fatalf("created=%d duplicates=%d, want one of each", created, duplicates)
	}
	stored, err := repository.ParcelByCustomerIdempotencyKey(ctx, *first.CustomerUserID, key)
	if err != nil {
		t.Fatalf("lookup winning request: %v", err)
	}
	if stored.ID != first.ID && stored.ID != second.ID {
		t.Fatal("lookup returned a parcel outside the concurrent requests")
	}
}

func TestParcelRepositorySharesCallerTransaction(t *testing.T) {
	database := parceltest.New(t)
	reader := testRepository(t, database.Pool)
	for _, commit := range []bool{false, true} {
		name := "rollback"
		if commit {
			name = "commit"
		}
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			tx, err := database.Pool.Begin(ctx)
			if err != nil {
				t.Fatalf("begin shared transaction: %v", err)
			}
			t.Cleanup(func() {
				if err := tx.Rollback(context.Background()); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
					t.Errorf("rollback shared transaction: %v", err)
				}
			})
			writer := testRepository(t, tx)
			params := testParcelParams(t)
			created, err := writer.CreateParcel(ctx, params)
			if err != nil {
				t.Fatalf("create inside transaction: %v", err)
			}
			role := "CUSTOMER"
			initial, err := writer.AppendStatusHistory(ctx, AppendStatusHistoryParams{
				ID: parceltest.UUID(t), ParcelID: created.ID, NewStatus: ParcelStatusCreated,
				ActorUserID: params.CustomerUserID, ActorRole: &role,
			})
			if err != nil {
				t.Fatalf("append initial history: %v", err)
			}
			if initial.PreviousStatus != nil || initial.NewStatus != ParcelStatusCreated {
				t.Fatal("initial history should start at CREATED with no previous status")
			}
			eventID := parceltest.UUID(t)
			// These are the existing baseline's names, not a new producer contract.
			if _, err := tx.Exec(ctx, `INSERT INTO parcel.outbox_events (id, event_type, routing_key, payload)
				VALUES ($1, 'parcel.created', 'parcel.created', '{}')`, eventID); err != nil {
				t.Fatalf("insert outbox fixture in caller transaction: %v", err)
			}
			if _, err := reader.ParcelByID(ctx, created.ID); !errors.Is(err, ErrNotFound) {
				t.Fatalf("uncommitted parcel became visible: %v", err)
			}
			if commit {
				err = tx.Commit(ctx)
			} else {
				err = tx.Rollback(ctx)
			}
			if err != nil {
				t.Fatalf("finish caller transaction: %v", err)
			}
			_, lookupErr := reader.ParcelByID(ctx, created.ID)
			if commit && lookupErr != nil {
				t.Fatalf("committed parcel missing: %v", lookupErr)
			}
			if !commit && !errors.Is(lookupErr, ErrNotFound) {
				t.Fatalf("rolled-back parcel lookup = %v, want ErrNotFound", lookupErr)
			}
			history, err := reader.StatusHistoryByParcel(ctx, created.ID)
			if err != nil {
				t.Fatalf("read committed history: %v", err)
			}
			wantCount := 0
			if commit {
				wantCount = 1
			}
			if len(history) != wantCount {
				t.Fatalf("history count = %d, want %d", len(history), wantCount)
			}
			var outboxCount int
			if err := database.Pool.QueryRow(ctx, `SELECT count(*) FROM parcel.outbox_events WHERE id = $1`, eventID).Scan(
				&outboxCount,
			); err != nil {
				t.Fatalf("read outbox fixture: %v", err)
			}
			if outboxCount != wantCount {
				t.Fatalf("outbox count = %d, want %d", outboxCount, wantCount)
			}
		})
	}
}

func TestParcelRepositoryHistoryOrderingAndMetadata(t *testing.T) {
	database := parceltest.New(t)
	repository := testRepository(t, database.Pool)
	ctx := context.Background()
	created, err := repository.CreateParcel(ctx, testParcelParams(t))
	if err != nil {
		t.Fatalf("create fixture parcel: %v", err)
	}
	for _, entry := range []struct {
		previous string
		status   string
		when     string
	}{
		{previous: "CREATED", status: "FAILED", when: "2026-11-01T10:01:00Z"},
		{status: "CREATED", when: "2026-11-01T10:00:00Z"},
	} {
		if _, err := database.Pool.Exec(ctx, `INSERT INTO parcel.parcel_status_history
			(id, parcel_id, previous_status, new_status, failure_reason, created_at)
			VALUES ($1, $2, NULLIF($3, ''), $4::varchar(20),
			CASE WHEN $4::varchar(20) = 'FAILED' THEN 'recipient unavailable' END, $5::timestamptz)`,
			parceltest.UUID(t), created.ID, entry.previous, entry.status, entry.when,
		); err != nil {
			t.Fatalf("insert history ordering fixture: %v", err)
		}
	}
	history, err := repository.StatusHistoryByParcel(ctx, created.ID)
	if err != nil || len(history) != 2 {
		t.Fatalf("read history: count=%d, error=%v", len(history), err)
	}
	if history[0].NewStatus != ParcelStatusCreated || history[1].NewStatus != ParcelStatusFailed {
		t.Fatal("history is not ordered by occurrence time")
	}
	if history[1].PreviousStatus == nil || *history[1].PreviousStatus != ParcelStatusCreated {
		t.Fatal("previous status did not round-trip")
	}
	if _, err := database.Pool.Exec(ctx, `UPDATE parcel.parcels
		SET status = 'FAILED', failed_at = now(), failure_reason = 'recipient unavailable'
		WHERE id = $1`, created.ID); err != nil {
		t.Fatalf("set failure metadata fixture: %v", err)
	}
	failed, err := repository.ParcelByID(ctx, created.ID)
	if err != nil || failed.FailedAt == nil || failed.FailureReason == nil {
		t.Fatalf("failure metadata did not round-trip: %v", err)
	}
	delivery, err := repository.CreateParcel(ctx, testParcelParams(t))
	if err != nil {
		t.Fatalf("create delivery fixture: %v", err)
	}
	for _, status := range []ParcelStatus{ParcelStatusPickedUp, ParcelStatusInTransit, ParcelStatusOutForDelivery} {
		_, err := database.Pool.Exec(ctx,
			`UPDATE parcel.parcels SET status = $2 WHERE id = $1`, delivery.ID, status,
		)
		if err != nil {
			t.Fatalf("advance delivery fixture: %v", err)
		}
	}
	if _, err := database.Pool.Exec(ctx, `UPDATE parcel.parcels
		SET status = 'DELIVERED', delivered_at = now(), delivered_to_name = 'Accepting recipient'
		WHERE id = $1`, delivery.ID); err != nil {
		t.Fatalf("set delivery metadata fixture: %v", err)
	}
	delivered, err := repository.ParcelByID(ctx, delivery.ID)
	if err != nil || delivered.DeliveredAt == nil || delivered.DeliveredToName == nil {
		t.Fatalf("delivery metadata did not round-trip: %v", err)
	}
}

func testRepository(t *testing.T, db DB) *Repository {
	t.Helper()
	repository, err := New(db)
	if err != nil {
		t.Fatalf("New(): %v", err)
	}
	return repository
}

func testParcelParams(t *testing.T) CreateParcelParams {
	t.Helper()
	parcelID := parceltest.UUID(t)
	customerID := parceltest.UUID(t)
	return CreateParcelParams{
		ID: parcelID, TrackingNumber: "SDK" + strings.ToUpper(strings.ReplaceAll(parcelID, "-", "")[:8]),
		CustomerUserID: &customerID, SenderName: "Sender", SenderPhone: "+94770000001",
		PickupAddress: "Pickup", PickupCity: "Colombo", PickupZoneID: parceltest.UUID(t),
		RecipientName: "Recipient", RecipientPhone: "+94770000002",
		DropoffAddress: "Dropoff", DropoffCity: "Kandy", DropoffZoneID: parceltest.UUID(t),
		WeightKG: testNumeric(t, "2.10"), SizeCategory: SizeCategorySmall,
		PriceAmount: testNumeric(t, "550.25"), Currency: "LKR",
		PriceBreakdown:  json.RawMessage(`{"base_fee":450.25,"weight_charge":100,"size_surcharge":0}`),
		CreatedByUserID: customerID,
	}
}

func testNumeric(t *testing.T, value string) pgtype.Numeric {
	t.Helper()
	var number pgtype.Numeric
	if err := number.Scan(value); err != nil {
		t.Fatalf("parse numeric fixture: %v", err)
	}
	return number
}
