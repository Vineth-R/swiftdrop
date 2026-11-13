//go:build integration

package database_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestMerchantOrderReservationMigration(t *testing.T) {
	t.Run("a fresh database has the reservation table and constraints", func(t *testing.T) {
		ctx, conn := newIsolatedDatabase(t)
		applyMerchantThrough(t, ctx, conn, 8)
		assertReservationSchema(t, ctx, conn, true)
	})

	t.Run("an existing M8 database keeps its orders and gains the reservation table", func(t *testing.T) {
		ctx, conn := newIsolatedDatabase(t)
		applyMerchantThrough(t, ctx, conn, 7)
		merchantID := insertPreM8Merchant(t, ctx, conn)
		var orderID string
		if err := conn.QueryRow(ctx, `
			INSERT INTO merchant.merchant_orders (
				merchant_id, external_order_id, request_hash, parcel_id, tracking_number,
				recipient_name, dropoff_city, weight_kg, size_category, fee_amount,
				fee_breakdown, last_known_status, last_status_at
			) VALUES (
				$1, 'PRE-RESERVATION-1', repeat('c', 64), gen_random_uuid(), 'SDN00000003',
				'Old Recipient', 'Colombo', 1.5, 'SMALL', 550.00,
				'{}'::jsonb, 'CREATED', now()
			) RETURNING id::text`, merchantID,
		).Scan(&orderID); err != nil {
			t.Fatalf("insert existing order: %v", err)
		}

		applySQLFile(t, ctx, conn, "migrations/merchant/000011_merchant_order_idempotency.up.sql")
		assertReservationSchema(t, ctx, conn, true)

		var remaining int
		if err := conn.QueryRow(ctx, `SELECT count(*) FROM merchant.merchant_orders WHERE id = $1`, orderID).Scan(&remaining); err != nil || remaining != 1 {
			t.Fatalf("existing order was not preserved: count=%d err=%v", remaining, err)
		}
	})

	t.Run("rollback removes only the reservation table and reapply restores it", func(t *testing.T) {
		ctx, conn := newIsolatedDatabase(t)
		applyMerchantThrough(t, ctx, conn, 8)
		merchantID := insertPreM8Merchant(t, ctx, conn)
		var orderID string
		if err := conn.QueryRow(ctx, `
			INSERT INTO merchant.merchant_orders (
				merchant_id, external_order_id, request_hash, parcel_id, tracking_number,
				recipient_name, dropoff_city, weight_kg, size_category, fee_amount,
				fee_breakdown, last_known_status, last_status_at
			) VALUES (
				$1, 'PRE-RESERVATION-2', repeat('d', 64), gen_random_uuid(), 'SDN00000004',
				'Old Recipient', 'Kandy', 2.0, 'MEDIUM', 750.00,
				'{}'::jsonb, 'IN_TRANSIT', now()
			) RETURNING id::text`, merchantID,
		).Scan(&orderID); err != nil {
			t.Fatalf("insert order before rollback: %v", err)
		}

		applySQLFile(t, ctx, conn, "migrations/merchant/000011_merchant_order_idempotency.down.sql")
		assertReservationSchema(t, ctx, conn, false)
		var remaining int
		if err := conn.QueryRow(ctx, `SELECT count(*) FROM merchant.merchant_orders WHERE id = $1`, orderID).Scan(&remaining); err != nil || remaining != 1 {
			t.Fatalf("rollback removed the order row: count=%d err=%v", remaining, err)
		}

		applySQLFile(t, ctx, conn, "migrations/merchant/000011_merchant_order_idempotency.up.sql")
		assertReservationSchema(t, ctx, conn, true)
	})
}

func assertReservationSchema(t *testing.T, ctx context.Context, conn *pgx.Conn, present bool) {
	t.Helper()
	var tableExists bool
	if err := conn.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.tables
			WHERE table_schema = 'merchant' AND table_name = 'merchant_order_reservations'
		)`).Scan(&tableExists); err != nil {
		t.Fatalf("read reservation table: %v", err)
	}
	if tableExists != present {
		t.Fatalf("reservation table exists = %v, want %v", tableExists, present)
	}
	var indexExists bool
	if err := conn.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM pg_indexes
			WHERE schemaname = 'merchant' AND tablename = 'merchant_order_reservations'
			  AND indexname = 'merchant_order_reservations_status_idx'
		)`).Scan(&indexExists); err != nil {
		t.Fatalf("read reservation index: %v", err)
	}
	if indexExists != present {
		t.Fatalf("reservation index exists = %v, want %v", indexExists, present)
	}
	var constraintExists bool
	if err := conn.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM pg_constraint
			WHERE conname = 'merchant_order_reservations_key_uq'
		)`).Scan(&constraintExists); err != nil {
		t.Fatalf("read reservation constraint: %v", err)
	}
	if constraintExists != present {
		t.Fatalf("reservation unique constraint exists = %v, want %v", constraintExists, present)
	}
}
