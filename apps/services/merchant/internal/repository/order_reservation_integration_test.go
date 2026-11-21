//go:build integration

package repository

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/orders"
)

type reservationZones struct{ name string }

func (z reservationZones) ZoneName(context.Context, string) (string, error) { return z.name, nil }

type cancellingParcel struct {
	cancel context.CancelFunc
	result orders.ParcelResult
	calls  int
}

func (p *cancellingParcel) CreateMerchant(context.Context, orders.ParcelInput) (orders.ParcelResult, error) {
	p.calls++
	if p.cancel != nil {
		cancel := p.cancel
		p.cancel = nil
		cancel()
	}
	return p.result, nil
}

func TestOrderCreationRecoversAfterDatabaseFailureFollowingParcel(t *testing.T) {
	ctx, pool, repo := newOrderTestPool(t)
	merchantID, ownerID := createOrderMerchant(t, ctx, repo)
	profile := orders.MerchantProfile{
		ID: merchantID, OwnerUserID: ownerID, BusinessName: "Reservation Shop",
		ContactPhone: "+94771234567", PickupAddress: "12 Galle Road",
		PickupZoneID: orderUUID(),
	}
	parcel := &cancellingParcel{result: orders.ParcelResult{
		ParcelID: orderUUID(), TrackingNumber: "SDN00000001", Status: "CREATED",
		Fee: orders.Fee{Currency: "LKR", BaseFee: 45000, WeightCharge: 10000, Total: 55000},
	}}
	service := orders.New(repo, parcel, reservationZones{name: "Colombo"})
	input := orders.CreateInput{
		ExternalOrderID: "SZ-DB-FAIL",
		Recipient: orders.Contact{
			Name: "Amali", Phone: "+94712345678", Address: "45 Lake Drive",
			City: "Kandy", ZoneID: orderUUID(),
		},
		WeightKG: 150, SizeCategory: "SMALL",
	}

	requestCtx, cancel := context.WithCancel(ctx)
	parcel.cancel = cancel
	if _, err := service.Create(requestCtx, profile, "db-fail-key", input); err == nil {
		t.Fatal("Create() after Parcel succeeded with a cancelled request context")
	}
	assertOrderReservationStatus(t, ctx, pool, merchantID, "db-fail-key", orders.OrderReservationFailed)

	different := input
	different.ExternalOrderID = "SZ-DB-FAIL-OTHER"
	if _, err := service.Create(ctx, profile, "db-fail-key", different); !errors.Is(err, orders.ErrDuplicateOrder) {
		t.Fatalf("different payload after database failure error = %v, want ErrDuplicateOrder", err)
	}
	if parcel.calls != 1 {
		t.Fatalf("different payload reached Parcel: calls = %d", parcel.calls)
	}

	result, err := service.Create(ctx, profile, "db-fail-key", input)
	if err != nil {
		t.Fatalf("retry after database failure error = %v", err)
	}
	if !result.Created {
		t.Fatalf("retry result = %+v, want a created order", result)
	}
	assertOrderReservationStatus(t, ctx, pool, merchantID, "db-fail-key", orders.OrderReservationCompleted)

	var orderCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM merchant.merchant_orders
		WHERE merchant_id = $1 AND idempotency_key = $2`, merchantID, "db-fail-key").Scan(&orderCount); err != nil {
		t.Fatalf("count orders: %v", err)
	}
	if orderCount != 1 {
		t.Fatalf("merchant orders = %d, want 1", orderCount)
	}
}

func TestOrderReservationLifecycle(t *testing.T) {
	ctx, pool, repo := newOrderTestPool(t)
	merchantID, _ := createOrderMerchant(t, ctx, repo)

	reservation, err := repo.ReserveOrderAttempt(ctx, merchantID, "lifecycle-key", orderHex(32))
	if err != nil {
		t.Fatalf("ReserveOrderAttempt() error = %v", err)
	}
	if reservation.Status != orders.OrderReservationPending {
		t.Fatalf("reservation status = %q", reservation.Status)
	}
	same, err := repo.ReserveOrderAttempt(ctx, merchantID, "lifecycle-key", reservation.RequestHash)
	if err != nil || same.ID != reservation.ID {
		t.Fatalf("same hash reservation = %+v, %v", same, err)
	}
	if _, err := repo.ReserveOrderAttempt(ctx, merchantID, "lifecycle-key", orderHex(32)); !errors.Is(err, orders.ErrDuplicateOrder) {
		t.Fatalf("different hash error = %v, want ErrDuplicateOrder", err)
	}
	if err := repo.FailOrderAttempt(ctx, reservation.ID, "parcel_unavailable"); err != nil {
		t.Fatalf("FailOrderAttempt() error = %v", err)
	}
	assertOrderReservationStatus(t, ctx, pool, merchantID, "lifecycle-key", orders.OrderReservationFailed)

	order, err := repo.CreateOrder(ctx, orderParams(merchantID, "SZ-LIFECYCLE"))
	if err != nil {
		t.Fatalf("CreateOrder() error = %v", err)
	}
	if order.ID == "" {
		t.Fatal("CreateOrder returned no order")
	}
	if err := repo.CompleteOrderAttempt(ctx, reservation.ID, order.ID); err != nil {
		t.Fatalf("CompleteOrderAttempt() error = %v", err)
	}
	assertOrderReservationStatus(t, ctx, pool, merchantID, "lifecycle-key", orders.OrderReservationCompleted)
}

func assertOrderReservationStatus(t *testing.T, ctx context.Context, pool *pgxpool.Pool, merchantID, key, want string) {
	t.Helper()
	var status string
	var orderID *string
	if err := pool.QueryRow(ctx, `SELECT status, order_id::text
		FROM merchant.merchant_order_reservations
		WHERE merchant_id = $1 AND idempotency_key = $2`, merchantID, key).Scan(&status, &orderID); err != nil {
		t.Fatalf("read reservation: %v", err)
	}
	if status != want {
		t.Fatalf("reservation status = %q, want %q", status, want)
	}
	if want == orders.OrderReservationCompleted && orderID == nil {
		t.Fatal("completed reservation has no order id")
	}
}
