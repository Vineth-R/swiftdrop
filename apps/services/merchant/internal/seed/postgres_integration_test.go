//go:build integration

package seed

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/repository"
	"github.com/Vineth-R/swiftdrop/packages/go-common/postgres"
)

const seedIntegrationTimeout = 120 * time.Second

func newSeedDatabase(t *testing.T) (context.Context, *postgres.Pool) {
	t.Helper()

	serverURL := os.Getenv("TEST_DATABASE_URL")
	if serverURL == "" {
		t.Skip("TEST_DATABASE_URL is not set; skipping merchant seed PostgreSQL integration tests")
	}

	ctx, cancel := context.WithTimeout(context.Background(), seedIntegrationTimeout)
	t.Cleanup(cancel)

	admin, err := pgx.Connect(ctx, serverURL)
	if err != nil {
		t.Fatalf("connect to test database server: %v", err)
	}

	databaseName := "swiftdrop_merchant_seed_" + randomHex(8)
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

	parsed, err := url.Parse(serverURL)
	if err != nil {
		t.Fatalf("parse TEST_DATABASE_URL: %v", err)
	}
	parsed.Path = "/" + databaseName

	config := postgres.DefaultConfig()
	config.URL = parsed.String()
	config.MaxConns = 8
	config.MinConns = 0
	config.RetryAttempts = 1
	pool, err := postgres.Open(ctx, config)
	if err != nil {
		t.Fatalf("open isolated database pool: %v", err)
	}
	t.Cleanup(pool.Close)

	for _, file := range []string{
		"infra/docker/postgres/init/01-schemas.sql",
		"migrations/pricing/000001_initial_schema.up.sql",
		"migrations/pricing/000002_tables.up.sql",
		"migrations/merchant/000001_initial_schema.up.sql",
		"migrations/merchant/000002_tables.up.sql",
		"migrations/merchant/000006_merchant_constraints.up.sql",
		"migrations/merchant/000007_registration_onboarding.up.sql",
		"migrations/merchant/000008_business_reg_no_optional.up.sql",
		"migrations/merchant/000009_restore_business_reg_no_required.up.sql",
	} {
		script, err := os.ReadFile(filepath.Join(seedRoot(t), filepath.FromSlash(file)))
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		if _, err := pool.Exec(ctx, string(script)); err != nil {
			t.Fatalf("apply %s: %v", file, err)
		}
	}
	return ctx, pool
}

func seedRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("find working directory: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find the repository root")
		}
		dir = parent
	}
}

func randomHex(size int) string {
	buffer := make([]byte, size)
	if _, err := rand.Read(buffer); err != nil {
		panic(fmt.Sprintf("read random bytes: %v", err))
	}
	return hex.EncodeToString(buffer)
}

func TestRunAgainstPostgresIsIdempotent(t *testing.T) {
	ctx, pool := newSeedDatabase(t)
	if _, err := pool.Exec(ctx, `INSERT INTO pricing.zones (code, name) VALUES ('CMB', 'Colombo') ON CONFLICT (code) DO NOTHING`); err != nil {
		t.Fatalf("insert pricing zone: %v", err)
	}

	config := testConfig(t)
	repo, err := repository.New(pool)
	if err != nil {
		t.Fatalf("repository.New returned error: %v", err)
	}
	state := &identityState{createCreated: true}
	identity := newTestIdentity(t, state)
	zones := NewPostgresZoneSource(pool)

	first, err := Run(ctx, repo, identity, zones, config)
	if err != nil {
		t.Fatalf("first Run returned error: %v", err)
	}
	if first.AlreadySeeded || first.Merchant.Status != repository.MerchantStatusApproved {
		t.Fatalf("first result = %+v, want a newly approved merchant", first)
	}
	if state.creates != 1 || state.lifecycles != 1 {
		t.Fatalf("identity calls = creates %d, lifecycles %d; want 1 each", state.creates, state.lifecycles)
	}

	second, err := Run(ctx, repo, identity, zones, config)
	if err != nil {
		t.Fatalf("second Run returned error: %v", err)
	}
	if !second.AlreadySeeded {
		t.Fatal("second Run did not report AlreadySeeded")
	}
	if !reflect.DeepEqual(second.Merchant, first.Merchant) {
		t.Fatalf("rerun changed the approved merchant: before=%+v after=%+v", first.Merchant, second.Merchant)
	}
	if state.creates != 1 || state.lifecycles != 1 {
		t.Fatalf("rerun called identity again: creates %d, lifecycles %d", state.creates, state.lifecycles)
	}

	lookup, err := repo.MerchantByContactEmail(ctx, strings.ToUpper(config.ContactEmail))
	if err != nil {
		t.Fatalf("MerchantByContactEmail returned error: %v", err)
	}
	if lookup.ID != first.Merchant.ID {
		t.Fatalf("MerchantByContactEmail returned %s, want %s", lookup.ID, first.Merchant.ID)
	}
}
