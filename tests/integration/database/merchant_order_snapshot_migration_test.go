//go:build integration

package database_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
)

var merchantSnapshotColumns = []string{
	"pickup_name",
	"pickup_phone",
	"pickup_address",
	"pickup_city",
	"pickup_zone_id",
	"recipient_phone",
	"recipient_address",
	"recipient_zone_id",
}

func TestMerchantOrderSnapshotMigration(t *testing.T) {
	t.Run("a fresh database has every snapshot column and the updated index", func(t *testing.T) {
		ctx, conn := newIsolatedDatabase(t)
		applyMerchantThrough(t, ctx, conn, len(merchantUpMigrations))
		assertMerchantSnapshotSchema(t, ctx, conn, true)
	})

	t.Run("pre-M8 rows survive and the columns stay nullable", func(t *testing.T) {
		ctx, conn := newIsolatedDatabase(t)
		applyMerchantThrough(t, ctx, conn, len(merchantUpMigrations)-1)
		merchantID := insertPreM8Merchant(t, ctx, conn)
		var orderID string
		if err := conn.QueryRow(ctx, `
			INSERT INTO merchant.merchant_orders (
				merchant_id, external_order_id, request_hash, parcel_id, tracking_number,
				recipient_name, dropoff_city, weight_kg, size_category, fee_amount,
				fee_breakdown, last_known_status, last_status_at
			) VALUES (
				$1, 'PRE-M8-1', repeat('a', 64), gen_random_uuid(), 'SDN00000001',
				'Old Recipient', 'Colombo', 1.5, 'SMALL', 550.00,
				'{}'::jsonb, 'CREATED', now()
			) RETURNING id::text`, merchantID,
		).Scan(&orderID); err != nil {
			t.Fatalf("insert pre-M8 order: %v", err)
		}

		applySQLFile(t, ctx, conn, "migrations/merchant/000010_merchant_order_snapshots.up.sql")
		assertMerchantSnapshotSchema(t, ctx, conn, true)

		var pickupName *string
		if err := conn.QueryRow(ctx,
			`SELECT pickup_name FROM merchant.merchant_orders WHERE id = $1`, orderID,
		).Scan(&pickupName); err != nil {
			t.Fatalf("read pre-M8 order after upgrade: %v", err)
		}
		if pickupName != nil {
			t.Fatalf("pre-M8 row acquired a pickup snapshot: %v", *pickupName)
		}
	})

	t.Run("rollback removes only the snapshot columns and index, then reapply restores them", func(t *testing.T) {
		ctx, conn := newIsolatedDatabase(t)
		applyMerchantThrough(t, ctx, conn, len(merchantUpMigrations))
		merchantID := insertPreM8Merchant(t, ctx, conn)
		var orderID string
		if err := conn.QueryRow(ctx, `
			INSERT INTO merchant.merchant_orders (
				merchant_id, external_order_id, request_hash, parcel_id, tracking_number,
				recipient_name, dropoff_city, weight_kg, size_category, fee_amount,
				fee_breakdown, last_known_status, last_status_at
			) VALUES (
				$1, 'PRE-M8-2', repeat('b', 64), gen_random_uuid(), 'SDN00000002',
				'Old Recipient', 'Kandy', 2.0, 'MEDIUM', 750.00,
				'{}'::jsonb, 'IN_TRANSIT', now()
			) RETURNING id::text`, merchantID,
		).Scan(&orderID); err != nil {
			t.Fatalf("insert order before rollback: %v", err)
		}

		applySQLFile(t, ctx, conn, "migrations/merchant/000010_merchant_order_snapshots.down.sql")
		assertMerchantSnapshotSchema(t, ctx, conn, false)
		var remaining int
		if err := conn.QueryRow(ctx,
			`SELECT count(*) FROM merchant.merchant_orders WHERE id = $1`, orderID,
		).Scan(&remaining); err != nil || remaining != 1 {
			t.Fatalf("rollback removed the order row: count=%d err=%v", remaining, err)
		}

		applySQLFile(t, ctx, conn, "migrations/merchant/000010_merchant_order_snapshots.up.sql")
		assertMerchantSnapshotSchema(t, ctx, conn, true)
	})
}

func assertMerchantSnapshotSchema(t *testing.T, ctx context.Context, conn *pgx.Conn, present bool) {
	t.Helper()
	for _, column := range merchantSnapshotColumns {
		var exists bool
		if err := conn.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM information_schema.columns
				WHERE table_schema = 'merchant' AND table_name = 'merchant_orders' AND column_name = $1
			)`, column).Scan(&exists); err != nil {
			t.Fatalf("read column %s: %v", column, err)
		}
		if exists != present {
			t.Fatalf("column %s exists = %v, want %v", column, exists, present)
		}
	}
	var indexExists bool
	if err := conn.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM pg_indexes
			WHERE schemaname = 'merchant' AND tablename = 'merchant_orders'
			  AND indexname = 'merchant_orders_updated_idx'
		)`).Scan(&indexExists); err != nil {
		t.Fatalf("read snapshot index: %v", err)
	}
	if indexExists != present {
		t.Fatalf("merchant_orders_updated_idx exists = %v, want %v", indexExists, present)
	}
}

func insertPreM8Merchant(t *testing.T, ctx context.Context, conn *pgx.Conn) string {
	t.Helper()
	var id string
	if err := conn.QueryRow(ctx, `
		INSERT INTO merchant.merchants (
			id, owner_user_id, business_name, business_reg_no, business_address,
			contact_email, contact_phone, pickup_address, pickup_zone_id, status
		) VALUES (
			gen_random_uuid(), gen_random_uuid(), 'Pre M8 Shop', 'PRE-M8-' || substr(md5(random()::text), 1, 8),
			'1 Test Road', 'pre-m8@example.test', '+94771234567', '1 Test Road',
			gen_random_uuid(), 'PENDING'
		) RETURNING id::text`).Scan(&id); err != nil {
		t.Fatalf("insert pre-M8 merchant: %v", err)
	}
	return id
}
