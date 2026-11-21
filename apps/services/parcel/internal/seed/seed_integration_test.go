//go:build integration && postgres

package seed

import (
	"context"
	"testing"

	"github.com/Vineth-R/swiftdrop/tests/integration/parceltest"
)

const (
	demoCustomerID   = "3f1c7a52-0000-4000-8000-000000000101"
	demoCourierOneID = "3f1c7a52-0000-4000-8000-000000000102"
	demoCourierTwoID = "3f1c7a52-0000-4000-8000-000000000103"
	demoDispatcherID = "3f1c7a52-0000-4000-8000-000000000104"
)

func TestRunCreatesSixStatusesAndIsIdempotent(t *testing.T) {
	fixture := parceltest.New(t)
	fixture.MigrateAll(t)
	ctx := context.Background()
	insertDemoIdentity(t, fixture)

	config := Config{
		AppEnv:          "development",
		DatabaseURL:     "postgres://demo@localhost/demo",
		CustomerEmail:   "seed.customer@example.com",
		CourierOneEmail: "seed.courier.1@example.com",
		CourierTwoEmail: "seed.courier.2@example.com",
		DispatcherEmail: "seed.dispatcher@example.com",
		TrackingPrefix:  "SDKDEMO",
	}

	first, err := Run(ctx, fixture.Pool, config)
	if err != nil {
		t.Fatalf("first Run: %v", err)
	}
	if first.Created != 6 || first.Skipped != 0 {
		t.Fatalf("first result = %+v", first)
	}

	second, err := Run(ctx, fixture.Pool, config)
	if err != nil {
		t.Fatalf("second Run: %v", err)
	}
	if second.Created != 0 || second.Skipped != 6 {
		t.Fatalf("second result = %+v", second)
	}

	var count int
	if err := fixture.Pool.QueryRow(ctx, `
		SELECT count(DISTINCT status) FROM parcel.parcels
		WHERE tracking_number LIKE 'SDKDEMO%'`).Scan(&count); err != nil {
		t.Fatalf("count statuses: %v", err)
	}
	if count != 6 {
		t.Fatalf("distinct statuses = %d, want 6", count)
	}

	for _, status := range []string{"CREATED", "PICKED_UP", "IN_TRANSIT", "OUT_FOR_DELIVERY", "DELIVERED", "FAILED"} {
		var found int
		if err := fixture.Pool.QueryRow(ctx,
			`SELECT count(*) FROM parcel.parcels WHERE tracking_number LIKE 'SDKDEMO%' AND status = $1`, status,
		).Scan(&found); err != nil {
			t.Fatalf("count %s: %v", status, err)
		}
		if found != 1 {
			t.Errorf("%s parcels = %d, want 1", status, found)
		}
	}
}

func insertDemoIdentity(t *testing.T, fixture *parceltest.Database) {
	t.Helper()
	ctx := context.Background()
	zoneID := ""
	if err := fixture.Pool.QueryRow(ctx, `SELECT id::text FROM pricing.zones WHERE code = 'CMB'`).Scan(&zoneID); err != nil {
		t.Fatalf("read zone: %v", err)
	}

	insert := func(id, email, role string) {
		if _, err := fixture.Pool.Exec(ctx, `
			INSERT INTO identity.users (id, email, password_hash, full_name, phone, role_id, auth_provider, email_verified)
			SELECT $1::uuid, $2, 'x', 'Demo', NULL, ro.id, 'LOCAL', true
			FROM identity.roles ro WHERE ro.name = $3`, id, email, role); err != nil {
			t.Fatalf("insert %s: %v", role, err)
		}
	}
	insert(demoCustomerID, "seed.customer@example.com", "CUSTOMER")
	insert(demoCourierOneID, "seed.courier.1@example.com", "COURIER")
	insert(demoCourierTwoID, "seed.courier.2@example.com", "COURIER")
	insert(demoDispatcherID, "seed.dispatcher@example.com", "DISPATCHER")

	for _, courierID := range []string{demoCourierOneID, demoCourierTwoID} {
		if _, err := fixture.Pool.Exec(ctx, `
			INSERT INTO identity.courier_profiles (user_id, vehicle_type, vehicle_number, home_zone_id)
			VALUES ($1::uuid, 'BIKE', 'SEED-1', $2::uuid)`, courierID, zoneID); err != nil {
			t.Fatalf("insert courier profile: %v", err)
		}
	}
}
