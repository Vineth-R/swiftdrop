//go:build integration

package repository

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Vineth-R/swiftdrop/packages/go-common/postgres"
)

// applyMigrationFile runs one migration file from the repository root against
// the isolated test database. newTestRepository already dropped the database
// when the test ends, so this never touches a shared database.
func applyMigrationFile(t *testing.T, pool *postgres.Pool, relativePath string) {
	t.Helper()
	script, err := os.ReadFile(filepath.Join(repositoryRoot(t), filepath.FromSlash(relativePath)))
	if err != nil {
		t.Fatalf("read %s: %v", relativePath, err)
	}
	if _, err := pool.Exec(context.Background(), string(script)); err != nil {
		t.Fatalf("apply %s: %v", relativePath, err)
	}
}

func oauthBindingColumnExists(t *testing.T, pool *postgres.Pool) bool {
	t.Helper()
	var exists bool
	if err := pool.QueryRow(context.Background(), `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_schema = 'identity'
			  AND table_name = 'oauth_states'
			  AND column_name = 'browser_binding_hash'
		)`).Scan(&exists); err != nil {
		t.Fatalf("read information_schema.columns: %v", err)
	}
	return exists
}

func oauthStatesTableExists(t *testing.T, pool *postgres.Pool) bool {
	t.Helper()
	var exists bool
	if err := pool.QueryRow(context.Background(), `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.tables
			WHERE table_schema = 'identity' AND table_name = 'oauth_states'
		)`).Scan(&exists); err != nil {
		t.Fatalf("read information_schema.tables: %v", err)
	}
	return exists
}

func TestIdentityMigration_rollbackAndReapply(t *testing.T) {
	_, pool := newTestRepository(t)

	if !oauthStatesTableExists(t, pool) || !oauthBindingColumnExists(t, pool) {
		t.Fatal("000001-000003 did not create oauth_states with browser_binding_hash")
	}

	// Roll back the browser-binding column, then the baseline tables, then the
	// schema. Reapply the whole chain and check it is usable again.
	applyMigrationFile(t, pool, "migrations/identity/000006_merchant_lifecycle_transitions.down.sql")
	applyMigrationFile(t, pool, "migrations/identity/000005_session_version.down.sql")
	applyMigrationFile(t, pool, "migrations/identity/000004_email_verification.down.sql")
	applyMigrationFile(t, pool, "migrations/identity/000003_oauth_states_browser_binding.down.sql")
	if oauthBindingColumnExists(t, pool) {
		t.Fatal("000003 down left browser_binding_hash behind")
	}

	applyMigrationFile(t, pool, "migrations/identity/000002_tables.down.sql")
	if oauthStatesTableExists(t, pool) {
		t.Fatal("000002 down left oauth_states behind")
	}

	applyMigrationFile(t, pool, "migrations/identity/000001_initial_schema.down.sql")
	applyMigrationFile(t, pool, "migrations/identity/000001_initial_schema.up.sql")
	applyMigrationFile(t, pool, "migrations/identity/000002_tables.up.sql")
	applyMigrationFile(t, pool, "migrations/identity/000003_oauth_states_browser_binding.up.sql")
	applyMigrationFile(t, pool, "migrations/identity/000004_email_verification.up.sql")
	applyMigrationFile(t, pool, "migrations/identity/000005_session_version.up.sql")
	applyMigrationFile(t, pool, "migrations/identity/000006_merchant_lifecycle_transitions.up.sql")

	if !oauthStatesTableExists(t, pool) || !oauthBindingColumnExists(t, pool) {
		t.Fatal("the reapplied migration chain did not restore oauth_states with browser_binding_hash")
	}
}
