//go:build integration

package repository

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Vineth-R/swiftdrop/packages/go-common/postgres"
)

const integrationTimeout = 60 * time.Second

// newTestRepository creates a uniquely named database on the server named by
// TEST_DATABASE_URL, applies the real schema bootstrap and identity migrations,
// and drops the database when the test ends. It never touches a shared
// application database.
func newTestRepository(t *testing.T) (*Repository, *postgres.Pool) {
	t.Helper()

	serverURL := os.Getenv("TEST_DATABASE_URL")
	if serverURL == "" {
		t.Skip("TEST_DATABASE_URL is not set; skipping PostgreSQL integration tests")
	}

	ctx, cancel := context.WithTimeout(context.Background(), integrationTimeout)
	t.Cleanup(cancel)

	admin, err := pgx.Connect(ctx, serverURL)
	if err != nil {
		t.Fatalf("connect to test database server: %v", err)
	}

	databaseName := "swiftdrop_identity_test_" + randomHex(8)
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

	root := repositoryRoot(t)
	for _, file := range []string{
		"infra/docker/postgres/init/01-schemas.sql",
		"migrations/identity/000001_initial_schema.up.sql",
		"migrations/identity/000002_tables.up.sql",
		"migrations/identity/000003_oauth_states_browser_binding.up.sql",
	} {
		script, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(file)))
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		if _, err := pool.Exec(ctx, string(script)); err != nil {
			t.Fatalf("apply %s: %v", file, err)
		}
	}

	repo, err := New(pool)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	return repo, pool
}

func repositoryRoot(t *testing.T) string {
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
		panic("read random bytes: " + err.Error())
	}
	return hex.EncodeToString(buffer)
}

// sha256Hex stands in for the service's refresh-token hash: CHAR(64) hex.
func sha256Hex(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
