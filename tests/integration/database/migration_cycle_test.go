//go:build integration

package database_test

import (
	"context"
	"net/url"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

type serviceMigrationCycle struct {
	service       string
	latestVersion int
	priorVersion  int
	latestPresent func(context.Context, *pgx.Conn) bool
}

// TestAllServiceMigrationCycles runs the real golang-migrate CLI against one
// disposable database for every service: fresh up, version check, down one,
// version and schema check, then up again. It verifies both the migration
// versions and the actual schema state, not only command exit codes.
func TestAllServiceMigrationCycles(t *testing.T) {
	ctx, conn, databaseURL := newIsolatedDatabaseWithURL(t)
	migrateBinary, err := exec.LookPath("migrate")
	if err != nil {
		t.Fatal("golang-migrate CLI is required on PATH for migration-cycle tests")
	}
	applySQLFile(t, ctx, conn, "infra/docker/postgres/init/01-schemas.sql")

	cycles := []serviceMigrationCycle{
		{
			service: "identity", latestVersion: 6, priorVersion: 5,
			latestPresent: func(ctx context.Context, conn *pgx.Conn) bool {
				return migrationObjectExists(t, ctx, conn, "identity.merchant_lifecycle_transitions")
			},
		},
		{
			service: "pricing", latestVersion: 2, priorVersion: 1,
			latestPresent: func(ctx context.Context, conn *pgx.Conn) bool {
				return migrationObjectExists(t, ctx, conn, "pricing.zones")
			},
		},
		{
			service: "parcel", latestVersion: 8, priorVersion: 7,
			latestPresent: func(ctx context.Context, conn *pgx.Conn) bool {
				return migrationColumnExists(t, ctx, conn, "parcel", "delivery_proofs", "proof_kind")
			},
		},
		{
			service: "merchant", latestVersion: 11, priorVersion: 10,
			latestPresent: func(ctx context.Context, conn *pgx.Conn) bool {
				return migrationObjectExists(t, ctx, conn, "merchant.merchant_order_reservations")
			},
		},
		{
			service: "notification", latestVersion: 3, priorVersion: 2,
			latestPresent: func(ctx context.Context, conn *pgx.Conn) bool {
				return migrationIndexExists(t, ctx, conn, "notification", "notification_logs_created_at_idx")
			},
		},
	}

	for _, cycle := range cycles {
		t.Run(cycle.service+" fresh up", func(t *testing.T) {
			runMigrate(t, migrateBinary, databaseURL, cycle.service, "up")
			assertMigrateVersion(t, migrateBinary, databaseURL, cycle.service, cycle.latestVersion)
			if !cycle.latestPresent(ctx, conn) {
				t.Fatalf("%s latest schema object is missing after fresh up", cycle.service)
			}
		})
	}

	for _, cycle := range cycles {
		t.Run(cycle.service+" down and reapply", func(t *testing.T) {
			runMigrate(t, migrateBinary, databaseURL, cycle.service, "down")
			assertMigrateVersion(t, migrateBinary, databaseURL, cycle.service, cycle.priorVersion)
			if cycle.latestPresent(ctx, conn) {
				t.Fatalf("%s latest schema object still exists after down", cycle.service)
			}

			runMigrate(t, migrateBinary, databaseURL, cycle.service, "up")
			assertMigrateVersion(t, migrateBinary, databaseURL, cycle.service, cycle.latestVersion)
			if !cycle.latestPresent(ctx, conn) {
				t.Fatalf("%s latest schema object is missing after reapply", cycle.service)
			}
		})
	}

	for _, cycle := range cycles {
		t.Run(cycle.service+" final clean version", func(t *testing.T) {
			assertMigrateVersion(t, migrateBinary, databaseURL, cycle.service, cycle.latestVersion)
		})
	}
}

func runMigrate(t *testing.T, binary, databaseURL, service, direction string) {
	t.Helper()
	queryURL, err := url.Parse(databaseURL)
	if err != nil {
		t.Fatalf("parse isolated database URL: %v", err)
	}
	query := queryURL.Query()
	query.Set("x-migrations-table", service+"_schema_migrations")
	queryURL.RawQuery = query.Encode()

	args := []string{
		"-path", filepath.Join("migrations", service),
		"-database", queryURL.String(),
		direction,
	}
	if direction == "down" {
		args = append(args, "1")
	}
	command := exec.Command(binary, args...)
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	command.Dir = root
	// Do not print subprocess output: it can contain connection diagnostics.
	if err := command.Run(); err != nil {
		t.Fatalf("%s migration %s failed: %v", service, direction, err)
	}
}

func assertMigrateVersion(t *testing.T, binary, databaseURL, service string, want int) {
	t.Helper()
	queryURL, err := url.Parse(databaseURL)
	if err != nil {
		t.Fatalf("parse isolated database URL: %v", err)
	}
	query := queryURL.Query()
	query.Set("x-migrations-table", service+"_schema_migrations")
	queryURL.RawQuery = query.Encode()

	command := exec.Command(binary,
		"-path", filepath.Join("migrations", service),
		"-database", queryURL.String(),
		"version",
	)
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	command.Dir = root
	output, err := command.CombinedOutput()
	text := strings.TrimSpace(string(output))
	if strings.Contains(strings.ToLower(text), "dirty") {
		t.Fatalf("%s migration version is dirty", service)
	}
	if err != nil {
		t.Fatalf("%s migration version check failed: %v", service, err)
	}
	fields := strings.Fields(text)
	if len(fields) == 0 {
		t.Fatalf("%s migration version output is empty", service)
	}
	version, err := strconv.Atoi(fields[0])
	if err != nil {
		t.Fatalf("%s migration version output is not numeric", service)
	}
	if version != want {
		t.Fatalf("%s migration version = %d, want %d", service, version, want)
	}
}

func migrationObjectExists(t *testing.T, ctx context.Context, conn *pgx.Conn, qualifiedName string) bool {
	t.Helper()
	var exists bool
	if err := conn.QueryRow(ctx, `SELECT to_regclass($1) IS NOT NULL`, qualifiedName).Scan(&exists); err != nil {
		t.Fatalf("check object %s: %v", qualifiedName, err)
	}
	return exists
}

func migrationColumnExists(t *testing.T, ctx context.Context, conn *pgx.Conn, schema, table, column string) bool {
	t.Helper()
	var exists bool
	if err := conn.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_schema = $1 AND table_name = $2 AND column_name = $3
		)`, schema, table, column).Scan(&exists); err != nil {
		t.Fatalf("check column %s.%s.%s: %v", schema, table, column, err)
	}
	return exists
}

func migrationIndexExists(t *testing.T, ctx context.Context, conn *pgx.Conn, schema, index string) bool {
	t.Helper()
	var exists bool
	if err := conn.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM pg_indexes WHERE schemaname = $1 AND indexname = $2
		)`, schema, index).Scan(&exists); err != nil {
		t.Fatalf("check index %s.%s: %v", schema, index, err)
	}
	return exists
}
