//go:build integration

package database_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const databaseTestTimeout = 120 * time.Second

var parcelUpMigrations = []string{
	"migrations/parcel/000001_initial_schema.up.sql",
	"migrations/parcel/000002_tables.up.sql",
	"migrations/parcel/000003_dispatch_foundations.up.sql",
}

var parcelDownMigrations = []string{
	"migrations/parcel/000003_dispatch_foundations.down.sql",
	"migrations/parcel/000002_tables.down.sql",
}

// newIsolatedDatabase creates a uniquely named database on the server named by
// TEST_DATABASE_URL and drops it when the test finishes. Tests never connect to
// a shared application database directly.
func newIsolatedDatabase(t *testing.T) (context.Context, *pgx.Conn) {
	t.Helper()

	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is not set; skipping PostgreSQL integration tests")
	}

	ctx, cancel := context.WithTimeout(context.Background(), databaseTestTimeout)
	t.Cleanup(cancel)

	adminConfig, err := pgx.ParseConfig(url)
	if err != nil {
		t.Fatalf("parse TEST_DATABASE_URL: %v", err)
	}
	admin, err := pgx.ConnectConfig(ctx, adminConfig)
	if err != nil {
		t.Fatalf("connect to test database server: %v", err)
	}

	databaseName := "swiftdrop_test_" + randomHex(8)
	quoted := pgx.Identifier{databaseName}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+quoted); err != nil {
		_ = admin.Close(ctx)
		t.Fatalf("create isolated database: %v", err)
	}

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		if _, err := admin.Exec(cleanupCtx, "DROP DATABASE IF EXISTS "+quoted+" WITH (FORCE)"); err != nil {
			t.Logf("drop isolated database %s: %v", databaseName, err)
		}
		_ = admin.Close(cleanupCtx)
	})

	testConfig, err := pgx.ParseConfig(url)
	if err != nil {
		t.Fatalf("parse TEST_DATABASE_URL for isolated database: %v", err)
	}
	testConfig.Database = databaseName
	conn, err := pgx.ConnectConfig(ctx, testConfig)
	if err != nil {
		t.Fatalf("connect to isolated database: %v", err)
	}
	t.Cleanup(func() {
		_ = conn.Close(context.Background())
	})

	return ctx, conn
}

func randomHex(bytes int) string {
	buf := make([]byte, bytes)
	if _, err := rand.Read(buf); err != nil {
		panic(fmt.Sprintf("read random bytes: %v", err))
	}
	return hex.EncodeToString(buf)
}

func newUUID(t *testing.T, ctx context.Context, conn *pgx.Conn) string {
	t.Helper()
	var id string
	if err := conn.QueryRow(ctx, `SELECT gen_random_uuid()::text`).Scan(&id); err != nil {
		t.Fatalf("generate UUID: %v", err)
	}
	return id
}

func repositoryPath(parts ...string) string {
	base := []string{"..", "..", ".."}
	return filepath.Join(append(base, parts...)...)
}

func applySQLFile(t *testing.T, ctx context.Context, conn *pgx.Conn, relativePath string) {
	t.Helper()
	sql, err := os.ReadFile(repositoryPath(relativePath))
	if err != nil {
		t.Fatalf("read %s: %v", relativePath, err)
	}
	if _, err := conn.PgConn().Exec(ctx, string(sql)).ReadAll(); err != nil {
		t.Fatalf("apply %s: %v", relativePath, err)
	}
}

func applyParcelMigrations(t *testing.T, ctx context.Context, conn *pgx.Conn, through int) {
	t.Helper()
	applySQLFile(t, ctx, conn, "infra/docker/postgres/init/01-schemas.sql")
	for version := 1; version <= through; version++ {
		applySQLFile(t, ctx, conn, parcelUpMigrations[version-1])
	}
}

func rollbackParcelMigrations(t *testing.T, ctx context.Context, conn *pgx.Conn, count int) {
	t.Helper()
	for index := 0; index < count; index++ {
		applySQLFile(t, ctx, conn, parcelDownMigrations[index])
	}
}

func insertParcel(t *testing.T, ctx context.Context, conn *pgx.Conn, status string) string {
	t.Helper()

	var failureReason *string
	var failedAt *time.Time
	var deliveredAt *time.Time
	var deliveredTo *string

	switch status {
	case "FAILED":
		reason := "Recipient unavailable after two attempts"
		now := time.Now()
		failureReason = &reason
		failedAt = &now
	case "DELIVERED":
		recipient := "Amali Fernando"
		now := time.Now()
		deliveredAt = &now
		deliveredTo = &recipient
	}

	trackingNumber := "SDK" + randomHex(4)
	var parcelID string
	err := conn.QueryRow(ctx, `
		INSERT INTO parcel.parcels (
			tracking_number, customer_user_id, sender_name, sender_phone,
			pickup_address, pickup_city, pickup_zone_id,
			recipient_name, recipient_phone, dropoff_address, dropoff_city, dropoff_zone_id,
			weight_kg, size_category, status, failure_reason,
			price_amount, price_breakdown, created_by_user_id,
			delivered_at, delivered_to_name, failed_at
		) VALUES (
			$1, gen_random_uuid(), 'Test Sender', '+94710000000',
			'1 Pickup Street', 'Colombo', gen_random_uuid(),
			'Amali Fernando', '+94710000001', '1 Dropoff Street', 'Kandy', gen_random_uuid(),
			2.00, 'SMALL', $2, $3,
			550.00, '{"base_fee": 300, "weight_charge": 150, "size_surcharge": 100}'::jsonb,
			gen_random_uuid(), $4, $5, $6
		)
		RETURNING id`,
		trackingNumber, status, failureReason, deliveredAt, deliveredTo, failedAt,
	).Scan(&parcelID)
	if err != nil {
		t.Fatalf("insert parcel with status %s: %v", status, err)
	}
	return parcelID
}

func insertAssignment(
	t *testing.T,
	ctx context.Context,
	conn *pgx.Conn,
	parcelID string,
	courierID string,
	previousCourierID *string,
	isCurrent bool,
) string {
	t.Helper()

	var assignmentID string
	err := conn.QueryRow(ctx, `
		INSERT INTO parcel.parcel_assignments (
			parcel_id, courier_id, previous_courier_id, assigned_by,
			is_current, unassigned_at, reason
		) VALUES (
			$1, $2, $3, gen_random_uuid(),
			$4, CASE WHEN $4 THEN NULL ELSE now() END, 'test assignment'
		)
		RETURNING id`,
		parcelID, courierID, previousCourierID, isCurrent,
	).Scan(&assignmentID)
	if err != nil {
		t.Fatalf("insert assignment: %v", err)
	}
	return assignmentID
}

func insertBaselineAssignment(
	t *testing.T,
	ctx context.Context,
	conn *pgx.Conn,
	parcelID string,
	courierID string,
	isCurrent bool,
) string {
	t.Helper()

	var assignmentID string
	err := conn.QueryRow(ctx, `
		INSERT INTO parcel.parcel_assignments (parcel_id, courier_id, assigned_by, is_current, unassigned_at)
		VALUES ($1, $2, gen_random_uuid(), $3, CASE WHEN $3 THEN NULL ELSE now() END)
		RETURNING id`,
		parcelID, courierID, isCurrent,
	).Scan(&assignmentID)
	if err != nil {
		t.Fatalf("insert baseline assignment: %v", err)
	}
	return assignmentID
}

func assertSQLState(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected SQLSTATE %s, got nil", want)
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		t.Fatalf("expected *pgconn.PgError, got %T (%v)", err, err)
	}
	if pgErr.Code != want {
		t.Fatalf("SQLSTATE = %s; want %s (%v)", pgErr.Code, want, pgErr)
	}
}

func columnType(t *testing.T, ctx context.Context, conn *pgx.Conn, table, column string) (string, string) {
	t.Helper()
	var dataType, nullable string
	err := conn.QueryRow(ctx, `
		SELECT data_type, is_nullable
		FROM information_schema.columns
		WHERE table_schema = 'parcel' AND table_name = $1 AND column_name = $2`,
		table, column,
	).Scan(&dataType, &nullable)
	if err != nil {
		t.Fatalf("column %s.%s: %v", table, column, err)
	}
	return dataType, nullable
}

func indexExists(t *testing.T, ctx context.Context, conn *pgx.Conn, table, index string) bool {
	t.Helper()
	var exists bool
	err := conn.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM pg_indexes
			WHERE schemaname = 'parcel' AND tablename = $1 AND indexname = $2
		)`, table, index).Scan(&exists)
	if err != nil {
		t.Fatalf("query index %s: %v", index, err)
	}
	return exists
}

func constraintExists(t *testing.T, ctx context.Context, conn *pgx.Conn, table, constraint string) bool {
	t.Helper()
	var exists bool
	err := conn.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM pg_constraint c
			JOIN pg_class t ON t.oid = c.conrelid
			JOIN pg_namespace n ON n.oid = t.relnamespace
			WHERE n.nspname = 'parcel' AND t.relname = $1 AND c.conname = $2
		)`, table, constraint).Scan(&exists)
	if err != nil {
		t.Fatalf("query constraint %s: %v", constraint, err)
	}
	return exists
}

func triggerExists(t *testing.T, ctx context.Context, conn *pgx.Conn, table, trigger string) bool {
	t.Helper()
	var exists bool
	err := conn.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM pg_trigger
			WHERE tgrelid = ('parcel.' || $1)::regclass AND tgname = $2 AND NOT tgisinternal
		)`, table, trigger).Scan(&exists)
	if err != nil {
		t.Fatalf("query trigger %s: %v", trigger, err)
	}
	return exists
}
