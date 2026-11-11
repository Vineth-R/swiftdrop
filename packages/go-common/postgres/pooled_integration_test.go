//go:build integration

package postgres_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/Vineth-R/swiftdrop/packages/go-common/postgres"
)

// TestPooledTransactionCompatibility exercises the query, transaction,
// advisory-xact-lock and prepared-statement paths through a PgBouncer
// transaction-mode endpoint. It is run by the pooled CI job and against a Neon
// staging pooled URL.
func TestPooledTransactionCompatibility(t *testing.T) {
	url := os.Getenv("TEST_POOLED_DATABASE_URL")
	if url == "" {
		t.Fatal("TEST_POOLED_DATABASE_URL is required for the pooled compatibility test")
	}
	config := postgres.DefaultConfig()
	config.URL = url
	config.MaxConns = 4
	config.MinConns = 0
	config.RetryAttempts = 1

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := postgres.Open(ctx, config)
	if err != nil {
		t.Fatalf("open pooled database: %v", err)
	}
	defer pool.Close()

	table := fmt.Sprintf("public.pooled_compat_%d", time.Now().UnixNano())
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_, _ = pool.Exec(cleanupCtx, "DROP TABLE IF EXISTS "+table)
	})

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin through pooler: %v", err)
	}
	if _, err := tx.Exec(ctx, "CREATE TABLE "+table+" (id INT PRIMARY KEY, note TEXT NOT NULL)"); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("create through pooler: %v", err)
	}
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", "swiftdrop-pooled"); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("transaction advisory lock through pooler: %v", err)
	}
	if _, err := tx.Exec(ctx, "INSERT INTO "+table+" (id,note) VALUES ($1,$2)", 1, "first"); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("insert through pooler: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit through pooler: %v", err)
	}

	// Repeat the same parameterized query so pgx reuses protocol-level
	// prepared statements across pooled transactions.
	for i := 0; i < 3; i++ {
		var note string
		if err := pool.QueryRow(ctx, "SELECT note FROM "+table+" WHERE id=$1", 1).Scan(&note); err != nil {
			t.Fatalf("prepared query %d through pooler: %v", i, err)
		}
		if note != "first" {
			t.Fatalf("query %d note = %q", i, note)
		}
	}

	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("readiness ping through pooler: %v", err)
	}
}
