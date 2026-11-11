//go:build integration

// Package eventflow proves the M1 event chain with real infrastructure:
// a courier status transaction writes the outbox, the real relay and AMQP
// publisher deliver to RabbitMQ, and the real notification, webhook and
// realtime handlers process the event. Merchant notifications use the real
// merchant internal lookup and client.
package eventflow

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Vineth-R/swiftdrop/packages/go-common/postgres"
)

const (
	testSetupTimeout = 120 * time.Second
	eventWaitTimeout = 30 * time.Second
)

var migrationFiles = []string{
	"infra/docker/postgres/init/01-schemas.sql",
	"migrations/parcel/000001_initial_schema.up.sql",
	"migrations/parcel/000002_tables.up.sql",
	"migrations/parcel/000003_dispatch_foundations.up.sql",
	"migrations/parcel/000004_price_breakdown_total.up.sql",
	"migrations/parcel/000005_outbox_event_types.up.sql",
	"migrations/parcel/000006_customer_parcel_list_index.up.sql",
	"migrations/notification/000001_initial_schema.up.sql",
	"migrations/notification/000002_tables.up.sql",
	"migrations/merchant/000001_initial_schema.up.sql",
	"migrations/merchant/000002_tables.up.sql",
	"migrations/merchant/000006_merchant_constraints.up.sql",
	"migrations/merchant/000007_registration_onboarding.up.sql",
	"migrations/merchant/000008_business_reg_no_optional.up.sql",
	"migrations/merchant/000009_restore_business_reg_no_required.up.sql",
}

// newIsolatedDatabase creates a uniquely named database on TEST_DATABASE_URL,
// applies the parcel, notification and merchant migrations, and drops only that
// database during cleanup. It returns a shared pool and the database URL.
func newIsolatedDatabase(t *testing.T) (*postgres.Pool, string) {
	t.Helper()

	rawURL := os.Getenv("TEST_DATABASE_URL")
	if rawURL == "" {
		t.Skip("TEST_DATABASE_URL is not set; skipping event-flow integration tests")
	}

	ctx, cancel := context.WithTimeout(context.Background(), testSetupTimeout)
	t.Cleanup(cancel)

	adminConfig, err := pgx.ParseConfig(rawURL)
	if err != nil {
		t.Fatalf("parse TEST_DATABASE_URL: %v", err)
	}
	admin, err := pgx.ConnectConfig(ctx, adminConfig)
	if err != nil {
		t.Fatalf("connect to test database server: %v", err)
	}

	databaseName := "swiftdrop_eventflow_" + randomHex(8)
	identifier := pgx.Identifier{databaseName}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+identifier); err != nil {
		_ = admin.Close(ctx)
		t.Fatalf("create isolated database: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		if _, err := admin.Exec(cleanupCtx, "DROP DATABASE IF EXISTS "+identifier+" WITH (FORCE)"); err != nil {
			t.Logf("drop isolated database %s: %v", databaseName, err)
		}
		_ = admin.Close(cleanupCtx)
	})

	testConfig, err := pgx.ParseConfig(rawURL)
	if err != nil {
		t.Fatalf("parse TEST_DATABASE_URL for isolated database: %v", err)
	}
	testConfig.Database = databaseName
	conn, err := pgx.ConnectConfig(ctx, testConfig)
	if err != nil {
		t.Fatalf("connect to isolated database: %v", err)
	}

	root := repositoryRoot(t)
	for _, relative := range migrationFiles {
		sql, readErr := os.ReadFile(filepath.Join(root, relative))
		if readErr != nil {
			_ = conn.Close(ctx)
			t.Fatalf("read %s: %v", relative, readErr)
		}
		if _, execErr := conn.PgConn().Exec(ctx, string(sql)).ReadAll(); execErr != nil {
			_ = conn.Close(ctx)
			t.Fatalf("apply %s: %v", relative, execErr)
		}
	}
	if err := conn.Close(ctx); err != nil {
		t.Fatalf("close migration connection: %v", err)
	}

	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse isolated database URL: %v", err)
	}
	parsedURL.Path = "/" + databaseName
	databaseURL := parsedURL.String()
	poolConfig, err := postgres.LoadConfigFromURL(databaseURL)
	if err != nil {
		t.Fatalf("build pool config: %v", err)
	}
	pool, err := postgres.Open(ctx, poolConfig)
	if err != nil {
		t.Fatalf("open isolated pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool, databaseURL
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	return root
}

func randomUUID() string {
	raw := randomHex(16)
	return raw[0:8] + "-" + raw[8:12] + "-" + raw[12:16] + "-" + raw[16:20] + "-" + raw[20:32]
}

func randomHex(size int) string {
	buffer := make([]byte, size)
	if _, err := rand.Read(buffer); err != nil {
		panic(fmt.Sprintf("read random bytes: %v", err))
	}
	return hex.EncodeToString(buffer)
}
