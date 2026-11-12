//go:build integration && postgres

package integration

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/Vineth-R/swiftdrop/tests/integration/parceltest"
)

func TestParcelMigrations(t *testing.T) {
	database := parceltest.New(t)
	ctx := context.Background()
	var version int
	var dirty bool
	if err := database.Pool.QueryRow(ctx, `SELECT version, dirty FROM public.parcel_schema_migrations`).Scan(
		&version, &dirty,
	); err != nil {
		t.Fatalf("read migration version: %v", err)
	}
	if version != 6 || dirty {
		t.Fatalf("migration state = %d dirty=%v, want version 6 clean", version, dirty)
	}
	tablesToCheck := []string{
		"parcels", "parcel_status_history", "parcel_assignments", "delivery_proofs", "outbox_events",
	}
	for _, table := range tablesToCheck {
		var exists bool
		err := database.Pool.QueryRow(ctx,
			`SELECT to_regclass($1) IS NOT NULL`, "parcel."+table,
		).Scan(&exists)
		if err != nil {
			t.Fatalf("inspect table: %v", err)
		}
		if !exists {
			t.Fatalf("parcel.%s is missing", table)
		}
	}
	var lastErrorType string
	if err := database.Pool.QueryRow(ctx, `
		SELECT data_type FROM information_schema.columns
		WHERE table_schema = 'parcel' AND table_name = 'outbox_events' AND column_name = 'last_error'`,
	).Scan(&lastErrorType); err != nil {
		t.Fatalf("inspect outbox last_error type: %v", err)
	}
	if lastErrorType != "text" {
		t.Fatalf("outbox_events.last_error type = %s; want text", lastErrorType)
	}
	for _, index := range []string{
		"parcel.idx_outbox_events_published_cleanup",
		"parcel.idx_parcel_assignments_courier_workload",
	} {
		var exists bool
		if err := database.Pool.QueryRow(ctx, `SELECT to_regclass($1) IS NOT NULL`, index).Scan(&exists); err != nil {
			t.Fatalf("inspect index %s: %v", index, err)
		}
		if !exists {
			t.Fatalf("index %s is missing", index)
		}
	}
	var crossSchemaKeys int
	if err := database.Pool.QueryRow(ctx, `SELECT count(*) FROM pg_constraint AS constraint_record
		JOIN pg_class AS owning_table ON owning_table.oid = constraint_record.conrelid
		JOIN pg_namespace AS owning_schema ON owning_schema.oid = owning_table.relnamespace
		JOIN pg_class AS referenced_table ON referenced_table.oid = constraint_record.confrelid
		JOIN pg_namespace AS referenced_schema ON referenced_schema.oid = referenced_table.relnamespace
		WHERE constraint_record.contype = 'f' AND owning_schema.nspname = 'parcel'
		AND referenced_schema.nspname <> 'parcel'`).Scan(&crossSchemaKeys); err != nil {
		t.Fatalf("inspect service boundaries: %v", err)
	}
	if crossSchemaKeys != 0 {
		t.Fatal("parcel schema must not have cross-service foreign keys")
	}
	database.Migrate(t, "up")
	database.Migrate(t, "down")
	var tables int
	if err := database.Pool.QueryRow(ctx,
		`SELECT count(*) FROM information_schema.tables WHERE table_schema = 'parcel'`,
	).Scan(&tables); err != nil {
		t.Fatalf("inspect constraint rollback: %v", err)
	}
	if tables != len(tablesToCheck) {
		t.Fatalf("constraint rollback left %d tables, want %d", tables, len(tablesToCheck))
	}
	// Roll back intervening owner migrations, if present, before the baseline.
	for parcelMigrationVersion(t, database) > 1 {
		database.Migrate(t, "down")
	}
	if err := database.Pool.QueryRow(ctx,
		`SELECT count(*) FROM information_schema.tables WHERE table_schema = 'parcel'`,
	).Scan(&tables); err != nil {
		t.Fatalf("inspect rollback: %v", err)
	}
	if tables != 0 {
		t.Fatalf("table rollback left %d parcel tables", tables)
	}
	database.Migrate(t, "down")
	var schemaExists bool
	if err := database.Pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM pg_namespace WHERE nspname = 'parcel')`,
	).Scan(&schemaExists); err != nil {
		t.Fatalf("inspect schema rollback: %v", err)
	}
	if schemaExists {
		t.Fatal("initial migration rollback left the parcel schema")
	}
	database.Migrate(t, "up")
	if err := database.Pool.QueryRow(ctx, `SELECT version, dirty FROM public.parcel_schema_migrations`).Scan(
		&version, &dirty,
	); err != nil {
		t.Fatalf("read reapplied migration version: %v", err)
	}
	if version != 6 || dirty {
		t.Fatalf("reapplied migration state = %d dirty=%v, want version 6 clean", version, dirty)
	}
}

func TestParcelServiceMigrationCompatibility(t *testing.T) {
	database := parceltest.New(t)
	// Start the complete chain before Parcel's schema exists, just as a fresh
	// scripts/migrate.sh run does, rather than relying on fixture setup order.
	for parcelMigrationVersion(t, database) > 0 {
		database.Migrate(t, "down")
	}
	database.MigrateAll(t)
	database.MigrateAll(t)
	expectedTables := map[string]int{
		"identity": 9, "pricing": 5, "parcel": 5, "merchant": 10, "notification": 2,
	}
	expectedVersions := map[string]int{
		"identity": 6, "pricing": 2, "parcel": 6, "merchant": 11, "notification": 2,
	}
	for service, expected := range expectedTables {
		t.Run(service, func(t *testing.T) {
			ctx := context.Background()
			var tables int
			if err := database.Pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.tables
				WHERE table_schema = $1 AND table_type = 'BASE TABLE'`, service).Scan(&tables); err != nil {
				t.Fatalf("inspect service tables: %v", err)
			}
			if tables != expected {
				t.Fatalf("%s tables = %d, want %d", service, tables, expected)
			}
			// These identifiers come only from the fixed service inventory above.
			migrationTable := pgx.Identifier{"public", service + "_schema_migrations"}.Sanitize()
			var version int
			var dirty bool
			if err := database.Pool.QueryRow(ctx, `SELECT version, dirty FROM `+migrationTable).Scan(
				&version, &dirty,
			); err != nil {
				t.Fatalf("inspect service migration state: %v", err)
			}
			if version != expectedVersions[service] || dirty {
				t.Fatalf("%s migration state = %d dirty=%v, want version %d clean", service, version, dirty, expectedVersions[service])
			}
			t.Logf("%s: %d business tables, clean migration version %d", service, tables, version)
		})
	}
}

func parcelMigrationVersion(t *testing.T, database *parceltest.Database) int {
	t.Helper()
	var version int
	err := database.Pool.QueryRow(context.Background(),
		`SELECT COALESCE((SELECT version FROM public.parcel_schema_migrations), 0)`,
	).Scan(&version)
	if err != nil {
		t.Fatalf("read parcel migration version: %v", err)
	}
	return version
}

func TestParcelOwnerAndMetadataConstraints(t *testing.T) {
	database := parceltest.New(t)
	customerID := parceltest.UUID(t)
	merchantID := parceltest.UUID(t)
	tests := []struct {
		name       string
		customer   any
		merchant   any
		tracking   string
		status     string
		reason     any
		finishedAt bool
		recipient  any
		want       string
	}{
		{name: "customer owner", customer: customerID, tracking: "SDKABCDEFGH", status: "CREATED"},
		{name: "merchant owner", merchant: merchantID, tracking: "SDNABCDEFGH", status: "CREATED"},
		{name: "no owner", tracking: "SDKABCDEFGH", status: "CREATED", want: "parcels_one_owner"},
		{
			name: "both owners", customer: customerID, merchant: merchantID,
			tracking: "SDKABCDEFGH", status: "CREATED", want: "parcels_one_owner",
		},
		{
			name: "wrong prefix", customer: customerID,
			tracking: "SDNABCDEFGH", status: "CREATED", want: "parcels_tracking_prefix",
		},
		{
			name: "failed requires reason", customer: customerID, tracking: "SDKABCDEFGH",
			status: "FAILED", finishedAt: true, want: "parcels_failed_fields",
		},
		{
			name: "failed reason too short", customer: customerID, tracking: "SDKABCDEFGH",
			status: "FAILED", reason: "late", finishedAt: true, want: "parcels_failed_fields",
		},
		{
			name: "failed requires timestamp", customer: customerID, tracking: "SDKABCDEFGH",
			status: "FAILED", reason: "recipient unavailable", want: "parcels_failed_fields",
		},
		{
			name: "valid failure", customer: customerID, tracking: "SDKABCDEFGH",
			status: "FAILED", reason: "recipient unavailable", finishedAt: true,
		},
		{
			name: "delivered requires name", customer: customerID, tracking: "SDKABCDEFGH",
			status: "DELIVERED", finishedAt: true, want: "parcels_delivered_fields",
		},
		{
			name: "delivered requires timestamp", customer: customerID, tracking: "SDKABCDEFGH",
			status: "DELIVERED", recipient: "Recipient", want: "parcels_delivered_fields",
		},
		{
			name: "valid delivery", customer: customerID, tracking: "SDKABCDEFGH",
			status: "DELIVERED", recipient: "Recipient", finishedAt: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tx := parcelTestTransaction(t, database)
			_, err := tx.Exec(context.Background(), parcelFixtureInsert,
				parceltest.UUID(t), test.tracking, test.customer, test.merchant, nil, nil,
				test.status, test.reason, test.finishedAt, test.recipient,
			)
			if test.want == "" {
				if err != nil {
					t.Fatalf("valid parcel rejected: %v", err)
				}
				return
			}
			assertParcelConstraint(t, err, "23514", test.want)
		})
	}
}

func TestParcelIdempotencyConstraints(t *testing.T) {
	database := parceltest.New(t)
	tests := []struct {
		name         string
		merchant     bool
		different    bool
		differentKey bool
		key          any
		want         string
	}{
		{name: "same customer duplicate", key: "request-key", want: "parcels_customer_idem_uq"},
		{name: "different customers may reuse key", different: true, key: "request-key"},
		{name: "same customer may use different keys", differentKey: true, key: "request-key"},
		{name: "customer may omit key"},
		{name: "same merchant duplicate", merchant: true, key: "external-order", want: "parcels_merchant_external_uq"},
		{name: "different merchants may reuse order", merchant: true, different: true, key: "external-order"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tx := parcelTestTransaction(t, database)
			owner := parceltest.UUID(t)
			for index := range 2 {
				var customerID, merchantID, externalOrderID, idempotencyKey any
				tracking := []string{"SDKABCDEFGH", "SDKABCDEFGJ"}[index]
				if test.different && index == 1 {
					owner = parceltest.UUID(t)
				}
				if test.merchant {
					merchantID, externalOrderID = owner, test.key
					tracking = strings.Replace(tracking, "SDK", "SDN", 1)
				} else {
					customerID, idempotencyKey = owner, test.key
					if test.differentKey && index == 1 {
						idempotencyKey = "another-request-key"
					}
				}
				_, err := tx.Exec(context.Background(), parcelFixtureInsert,
					parceltest.UUID(t), tracking, customerID, merchantID, externalOrderID,
					idempotencyKey, "CREATED", nil, false, nil,
				)
				if index == 1 && test.want != "" {
					assertParcelConstraint(t, err, "23505", test.want)
					continue
				}
				if err != nil {
					t.Fatalf("insert parcel %d: %v", index, err)
				}
			}
		})
	}
}

func TestParcelHistoryConstraints(t *testing.T) {
	database := parceltest.New(t)
	tests := []struct {
		name      string
		operation string
		code      string
	}{
		{name: "orphan history", operation: "orphan", code: "23503"},
		{name: "failed history needs reason", operation: "failed", code: "23514"},
		{name: "history cannot be updated", operation: "update", code: "P0001"},
		{name: "history cannot be deleted", operation: "delete", code: "P0001"},
		{name: "parcel deletion does not cascade", operation: "delete parcel", code: "23503"},
		{name: "invalid transition", operation: "transition", code: "23514"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tx := parcelTestTransaction(t, database)
			ctx := context.Background()
			parcelID := parceltest.UUID(t)
			if _, err := tx.Exec(ctx, parcelFixtureInsert,
				parcelID, "SDKABCDEFGH", parceltest.UUID(t), nil, nil, nil, "CREATED", nil, false, nil,
			); err != nil {
				t.Fatalf("insert fixture: %v", err)
			}
			historyID := parceltest.UUID(t)
			if _, err := tx.Exec(ctx,
				`INSERT INTO parcel.parcel_status_history (id, parcel_id, new_status) VALUES ($1, $2, 'CREATED')`,
				historyID, parcelID,
			); err != nil {
				t.Fatalf("insert initial history: %v", err)
			}
			var err error
			switch test.operation {
			case "orphan":
				_, err = tx.Exec(ctx,
					`INSERT INTO parcel.parcel_status_history (id, parcel_id, new_status) VALUES ($1, $2, 'CREATED')`,
					parceltest.UUID(t), parceltest.UUID(t),
				)
			case "failed":
				_, err = tx.Exec(ctx,
					`INSERT INTO parcel.parcel_status_history (id, parcel_id, new_status) VALUES ($1, $2, 'FAILED')`,
					parceltest.UUID(t), parcelID,
				)
			case "update":
				_, err = tx.Exec(ctx, `UPDATE parcel.parcel_status_history SET note = 'changed' WHERE id = $1`, historyID)
			case "delete":
				_, err = tx.Exec(ctx, `DELETE FROM parcel.parcel_status_history WHERE id = $1`, historyID)
			case "delete parcel":
				_, err = tx.Exec(ctx, `DELETE FROM parcel.parcels WHERE id = $1`, parcelID)
			case "transition":
				_, err = tx.Exec(ctx, `UPDATE parcel.parcels SET status = 'IN_TRANSIT' WHERE id = $1`, parcelID)
			}
			assertParcelConstraint(t, err, test.code, "")
		})
	}
}

func TestParcelReportingIndexes(t *testing.T) {
	database := parceltest.New(t)
	indexes := map[string]string{
		"parcels_customer_idx":    "(customer_user_id)",
		"parcels_merchant_idx":    "(merchant_id)",
		"parcels_courier_idx":     "(current_courier_id)",
		"parcels_status_created":  "(status, created_at)",
		"parcels_created_idx":     "(created_at)",
		"parcels_delivered_idx":   "(delivered_at)",
		"parcels_failed_idx":      "(failed_at)",
		"parcels_pickup_zone_idx": "(pickup_zone_id, created_at)",
		"history_parcel_idx":      "(parcel_id, created_at)",
		"history_status_created":  "(new_status, created_at)",
	}
	for name, columns := range indexes {
		t.Run(name, func(t *testing.T) {
			var definition string
			err := database.Pool.QueryRow(context.Background(),
				`SELECT indexdef FROM pg_indexes WHERE schemaname = 'parcel' AND indexname = $1`, name,
			).Scan(&definition)
			if err != nil {
				t.Fatalf("query index: %v", err)
			}
			if !strings.Contains(definition, columns) {
				t.Fatalf("index %s does not cover %s: %s", name, columns, definition)
			}
		})
	}
}

func TestParcelPriceBreakdownConstraints(t *testing.T) {
	database := parceltest.New(t)
	tests := []struct {
		name      string
		breakdown string
		valid     bool
	}{
		{
			name: "matching total", valid: true,
			breakdown: `{"base_fee":450,"weight_charge":100,"size_surcharge":0}`,
		},
		{name: "missing component", breakdown: `{"base_fee":450,"weight_charge":100}`},
		{name: "different total", breakdown: `{"base_fee":400,"weight_charge":100,"size_surcharge":0}`},
		{name: "null base fee", breakdown: `{"base_fee":null,"weight_charge":100,"size_surcharge":0}`},
		{name: "null weight charge", breakdown: `{"base_fee":450,"weight_charge":null,"size_surcharge":0}`},
		{name: "null size surcharge", breakdown: `{"base_fee":450,"weight_charge":100,"size_surcharge":null}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tx := parcelTestTransaction(t, database)
			ctx := context.Background()
			parcelID := parceltest.UUID(t)
			if _, err := tx.Exec(ctx, parcelFixtureInsert,
				parcelID, "SDKABCDEFGH", parceltest.UUID(t), nil, nil, nil, "CREATED", nil, false, nil,
			); err != nil {
				t.Fatalf("insert price fixture: %v", err)
			}
			_, err := tx.Exec(ctx, `UPDATE parcel.parcels SET price_breakdown = $2::jsonb WHERE id = $1`,
				parcelID, test.breakdown,
			)
			if test.valid {
				if err != nil {
					t.Fatalf("matching price breakdown rejected: %v", err)
				}
				return
			}
			assertParcelConstraint(t, err, "23514", "parcels_price_breakdown")
		})
	}
}

func TestParcelAssignmentCompatibility(t *testing.T) {
	database := parceltest.New(t)
	for _, name := range []string{"current assignment", "orphan assignment", "duplicate current assignment", "reassignment"} {
		t.Run(name, func(t *testing.T) {
			tx := parcelTestTransaction(t, database)
			ctx := context.Background()
			parcelID := parceltest.UUID(t)
			if _, err := tx.Exec(ctx, parcelFixtureInsert,
				parcelID, "SDKABCDEFGH", parceltest.UUID(t), nil, nil, nil, "CREATED", nil, false, nil,
			); err != nil {
				t.Fatalf("insert parcel fixture: %v", err)
			}
			assignmentID, courierID := parceltest.UUID(t), parceltest.UUID(t)
			targetParcelID := parcelID
			if name == "orphan assignment" {
				targetParcelID = parceltest.UUID(t)
			}
			_, err := tx.Exec(ctx, `INSERT INTO parcel.parcel_assignments
				(id, parcel_id, courier_id, assigned_by) VALUES ($1, $2, $3, $4)`,
				assignmentID, targetParcelID, courierID, parceltest.UUID(t),
			)
			if name == "orphan assignment" {
				assertParcelConstraint(t, err, "23503", "")
				return
			}
			if err != nil {
				t.Fatalf("insert current assignment: %v", err)
			}
			if name == "reassignment" {
				if _, err := tx.Exec(ctx, `UPDATE parcel.parcel_assignments
					SET is_current = false, unassigned_at = now() WHERE id = $1`, assignmentID); err != nil {
					t.Fatalf("close previous assignment: %v", err)
				}
			}
			if name == "duplicate current assignment" || name == "reassignment" {
				courierID = parceltest.UUID(t)
				_, err := tx.Exec(ctx, `INSERT INTO parcel.parcel_assignments
					(id, parcel_id, courier_id, assigned_by) VALUES ($1, $2, $3, $4)`,
					parceltest.UUID(t), parcelID, courierID, parceltest.UUID(t),
				)
				if name == "duplicate current assignment" {
					assertParcelConstraint(t, err, "23505", "assignments_one_current_uq")
					return
				}
				if err != nil {
					t.Fatalf("insert replacement assignment: %v", err)
				}
			}
			if _, err := tx.Exec(ctx, `UPDATE parcel.parcels SET current_courier_id = $2 WHERE id = $1`,
				parcelID, courierID,
			); err != nil {
				t.Fatalf("update current courier copy: %v", err)
			}
			var matches bool
			if err := tx.QueryRow(ctx, `SELECT parcels.current_courier_id = assignments.courier_id
				FROM parcel.parcels AS parcels JOIN parcel.parcel_assignments AS assignments
				ON assignments.parcel_id = parcels.id AND assignments.is_current
				WHERE parcels.id = $1`, parcelID).Scan(&matches); err != nil {
				t.Fatalf("check assignment relationship: %v", err)
			}
			if !matches {
				t.Fatal("current courier copy disagrees with the current assignment")
			}
		})
	}
}

func parcelTestTransaction(t *testing.T, database *parceltest.Database) pgx.Tx {
	t.Helper()
	tx, err := database.Pool.Begin(context.Background())
	if err != nil {
		t.Fatalf("begin constraint test: %v", err)
	}
	t.Cleanup(func() {
		if err := tx.Rollback(context.Background()); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			t.Errorf("rollback constraint test: %v", err)
		}
	})
	return tx
}

func assertParcelConstraint(t *testing.T, err error, code, constraint string) {
	t.Helper()
	var postgresError *pgconn.PgError
	if !errors.As(err, &postgresError) {
		t.Fatalf("expected PostgreSQL error %s, got %v", code, err)
	}
	if postgresError.Code != code {
		t.Fatalf("SQLSTATE = %s, want %s", postgresError.Code, code)
	}
	if constraint != "" && postgresError.ConstraintName != constraint {
		t.Fatalf("constraint = %s, want %s", postgresError.ConstraintName, constraint)
	}
}

const parcelFixtureInsert = `INSERT INTO parcel.parcels (
	id, tracking_number, customer_user_id, merchant_id, external_order_id, idempotency_key,
	sender_name, sender_phone, pickup_address, pickup_city, pickup_zone_id,
	recipient_name, recipient_phone, dropoff_address, dropoff_city, dropoff_zone_id,
	weight_kg, size_category, price_amount, price_breakdown, created_by_user_id,
	status, failure_reason, delivered_at, failed_at, delivered_to_name
) VALUES (
	$1, $2, $3, $4, $5, $6,
	'Sender', '+94770000001', 'Pickup', 'Colombo', '00000000-0000-4000-8000-000000000001',
	'Recipient', '+94770000002', 'Dropoff', 'Kandy', '00000000-0000-4000-8000-000000000002',
	2.00, 'SMALL', 550.00, '{"base_fee":450,"weight_charge":100,"size_surcharge":0}',
	'00000000-0000-4000-8000-000000000003', $7::varchar(20), $8,
	CASE WHEN $9 AND $7::varchar(20) = 'DELIVERED' THEN now() END,
	CASE WHEN $9 AND $7::varchar(20) = 'FAILED' THEN now() END, $10
)`

// TestParcelCustomerListIndex verifies the additive 000006 index exists and
// that the real customer-list shape can use it for the ordered page query.
func TestParcelCustomerListIndex(t *testing.T) {
	database := parceltest.New(t)
	ctx := context.Background()

	var exists bool
	if err := database.Pool.QueryRow(ctx,
		`SELECT to_regclass('parcel.idx_parcels_customer_created_id') IS NOT NULL`,
	).Scan(&exists); err != nil {
		t.Fatalf("inspect customer list index: %v", err)
	}
	if !exists {
		t.Fatal("parcel.idx_parcels_customer_created_id is missing")
	}

	const customer = "11111111-1111-4111-8111-111111111111"
	const originZone = "00000000-0000-4000-8000-000000000001"
	const destinationZone = "00000000-0000-4000-8000-000000000002"
	if _, err := database.Pool.Exec(ctx, `
		INSERT INTO parcel.parcels (
			tracking_number, customer_user_id, sender_name, sender_phone,
			pickup_address, pickup_city, pickup_zone_id, recipient_name,
			recipient_phone, dropoff_address, dropoff_city, dropoff_zone_id,
			weight_kg, size_category, price_amount, price_breakdown,
			created_by_user_id, created_at
		)
		SELECT 'SDK' || lpad(gs::text, 8, '0'), $1, 'Sender', '+94770000001',
			'Pickup', 'Colombo', $2, 'Recipient', '+94770000002',
			'Dropoff', 'Kandy', $3, 2.00, 'SMALL', 550.00,
			'{"base_fee":450,"weight_charge":100,"size_surcharge":0}',
			$1, now() - (gs || ' minutes')::interval
		FROM generate_series(1, 200) AS gs`, customer, originZone, destinationZone,
	); err != nil {
		t.Fatalf("insert customer list fixtures: %v", err)
	}

	tx, err := database.Pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin explain transaction: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	// Sequential scans and explicit sorts are disabled so the plan must prove
	// that the composite index can satisfy the customer filter and ordering
	// without an extra sort. With realistic volumes the planner chooses it on
	// cost as well.
	if _, err := tx.Exec(ctx, `SET LOCAL enable_seqscan = off`); err != nil {
		t.Fatalf("disable sequential scan: %v", err)
	}
	if _, err := tx.Exec(ctx, `SET LOCAL enable_sort = off`); err != nil {
		t.Fatalf("disable sort: %v", err)
	}
	rows, err := tx.Query(ctx, `
		EXPLAIN (COSTS OFF)
		SELECT id FROM parcel.parcels
		WHERE customer_user_id = $1
		  AND ($2 = '' OR status = $2)
		  AND ($3::timestamptz IS NULL OR created_at >= $3)
		  AND ($4::timestamptz IS NULL OR created_at < $4)
		ORDER BY created_at DESC, id DESC
		LIMIT $5 OFFSET $6`,
		customer, "", nil, nil, 20, 0,
	)
	if err != nil {
		t.Fatalf("explain customer list: %v", err)
	}
	var plan strings.Builder
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			rows.Close()
			t.Fatalf("scan explain line: %v", err)
		}
		plan.WriteString(line)
		plan.WriteByte('\n')
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate explain lines: %v", err)
	}
	if !strings.Contains(plan.String(), "idx_parcels_customer_created_id") {
		t.Fatalf("customer list did not use the composite index:\n%s", plan.String())
	}
}
