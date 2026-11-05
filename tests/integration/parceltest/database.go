//go:build integration && postgres

// Package parceltest provides isolated PostgreSQL fixtures for Member 5's tests.
package parceltest

import (
	"context"
	"crypto/rand"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Vineth-R/swiftdrop/packages/go-common/postgres"
)

type Database struct {
	Pool          *postgres.Pool
	ServerVersion string
	root          string
	url           string
	migrateBinary string
}

// New creates its own database, applies the real migrations with golang-migrate,
// and drops only that generated database during cleanup. The supplied login must
// have CREATEDB permission. Missing configuration fails these opt-in tests.
func New(t *testing.T) *Database {
	t.Helper()
	rawURL := os.Getenv("PARCEL_TEST_DATABASE_URL")
	if rawURL == "" {
		t.Fatal("PARCEL_TEST_DATABASE_URL is required for PostgreSQL integration tests")
	}
	parsedURL, err := url.Parse(rawURL)
	if err != nil || parsedURL == nil {
		t.Fatal("PARCEL_TEST_DATABASE_URL must be a PostgreSQL URL")
	}
	validScheme := parsedURL.Scheme == "postgres" || parsedURL.Scheme == "postgresql"
	if !validScheme || parsedURL.Host == "" {
		t.Fatal("PARCEL_TEST_DATABASE_URL must be a PostgreSQL URL with a host")
	}
	migrateBinary, err := exec.LookPath("migrate")
	if err != nil {
		t.Fatal("golang-migrate CLI is required on PATH for PostgreSQL integration tests")
	}
	root := repositoryRoot(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, rawURL)
	if err != nil {
		t.Fatal("connect PostgreSQL fixture administrator failed; check PARCEL_TEST_DATABASE_URL")
	}
	databaseName := "swiftdrop_parcel_test_" + strings.ReplaceAll(UUID(t), "-", "")
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{databaseName}.Sanitize()); err != nil {
		closeErr := admin.Close(ctx)
		if closeErr != nil {
			t.Error("close fixture administrator failed")
		}
		t.Fatal("create isolated PostgreSQL database failed; the test login needs CREATEDB permission")
	}

	database := &Database{root: root, migrateBinary: migrateBinary}
	t.Cleanup(func() {
		if database.Pool != nil {
			database.Pool.Close()
		}
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cleanupCancel()
		if _, err := admin.Exec(cleanupCtx, "DROP DATABASE "+pgx.Identifier{databaseName}.Sanitize()); err != nil {
			t.Errorf("drop isolated test database %s failed", databaseName)
		}
		if err := admin.Close(cleanupCtx); err != nil {
			t.Error("close fixture administrator failed")
		}
	})

	database.url, err = isolatedDatabaseURL(rawURL, databaseName)
	if err != nil {
		t.Fatal("construct isolated PostgreSQL URL failed")
	}
	config := postgres.DefaultConfig()
	config.URL = database.url
	config.MaxConns = 4
	config.MinConns = 0
	config.RetryAttempts = 1
	database.Pool, err = postgres.Open(ctx, config)
	if err != nil {
		t.Fatal("open isolated PostgreSQL pool failed")
	}
	var connectedDatabase string
	var serverVersionNumber int
	if err := database.Pool.QueryRow(ctx,
		`SELECT current_database(), current_setting('server_version_num')::integer, current_setting('server_version')`,
	).Scan(&connectedDatabase, &serverVersionNumber, &database.ServerVersion); err != nil {
		t.Fatal("verify isolated PostgreSQL database failed")
	}
	if connectedDatabase != databaseName {
		t.Fatal("refusing to bootstrap a database other than the generated test database")
	}
	if serverVersionNumber/10000 != 16 {
		t.Fatalf("PostgreSQL 16 is required for Parcel acceptance tests; connected to %s", database.ServerVersion)
	}
	t.Logf("acceptance database: PostgreSQL %s", database.ServerVersion)
	bootstrap, err := os.ReadFile(filepath.Join(database.root, "infra", "docker", "postgres", "init", "01-schemas.sql"))
	if err != nil {
		t.Fatalf("read database bootstrap: %v", err)
	}
	if _, err := database.Pool.Exec(ctx, string(bootstrap)); err != nil {
		t.Fatalf("bootstrap isolated database: %v", err)
	}
	database.Migrate(t, "up")
	return database
}

// URL returns the isolated database connection URL. Tests that need a dedicated
// session, such as a non-UTC timezone, can open their own connection with it.
func (database *Database) URL() string { return database.url }

// Migrate uses the same per-service migration table as scripts/migrate.sh.
// Down rolls back one version, matching the project's migration runner.
func (database *Database) Migrate(t *testing.T, direction string) {
	t.Helper()
	database.migrateService(t, "parcel", direction)
}

// MigrateAll follows the same service order as scripts/migrate.sh. It is used
// only on this fixture's generated database, never on the maintenance database.
func (database *Database) MigrateAll(t *testing.T) {
	t.Helper()
	for _, service := range []string{"identity", "pricing", "parcel", "merchant", "notification"} {
		database.migrateService(t, service, "up")
	}
}

func (database *Database) migrateService(t *testing.T, service, direction string) {
	t.Helper()
	if direction != "up" && direction != "down" {
		t.Fatal("unsupported test migration direction")
	}
	parsedURL, err := url.Parse(database.url)
	if err != nil {
		t.Fatal("parse isolated migration URL failed")
	}
	query := parsedURL.Query()
	query.Set("x-migrations-table", service+"_schema_migrations")
	parsedURL.RawQuery = query.Encode()
	args := []string{
		"-path", "migrations/" + service,
		"-database", parsedURL.String(), direction,
	}
	if direction == "down" {
		args = append(args, "1")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	// Run from the repository root with a relative source path. The CLI turns
	// -path into file://path, which cannot parse an absolute Windows drive path.
	command := exec.CommandContext(ctx, database.migrateBinary, args...)
	command.Dir = database.root
	// Do not print subprocess output: driver diagnostics may include credentials.
	if err := command.Run(); err != nil {
		t.Fatalf("%s %s migration failed: %v", service, direction, err)
	}
}

func UUID(t *testing.T) string {
	t.Helper()
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		t.Fatalf("generate test UUID: %v", err)
	}
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", value[:4], value[4:6], value[6:8], value[8:10], value[10:])
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	root, err := os.Getwd()
	if err != nil {
		t.Fatalf("find test working directory: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(root, "go.mod")); err == nil {
			return root
		}
		parent := filepath.Dir(root)
		if parent == root {
			t.Fatal("could not find repository root")
		}
		root = parent
	}
}
