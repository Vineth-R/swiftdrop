//go:build integration

package database_test

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestNotificationLogRetentionMigration(t *testing.T) {
	ctx, conn := newIsolatedDatabase(t)
	applySQLFile(t, ctx, conn, "infra/docker/postgres/init/01-schemas.sql")
	applySQLFile(t, ctx, conn, "migrations/notification/000001_initial_schema.up.sql")
	applySQLFile(t, ctx, conn, "migrations/notification/000002_tables.up.sql")

	if notificationLogIndexExists(t, ctx, conn) {
		t.Fatal("created_at index exists before the retention migration")
	}

	if _, err := conn.Exec(ctx, `
		INSERT INTO notification.notification_logs
			(event_id, parcel_id, channel, recipient, status, created_at)
		VALUES (gen_random_uuid(), gen_random_uuid(), 'EMAIL', 'before-index@example.com', 'SENT', now())`); err != nil {
		t.Fatalf("insert existing notification log: %v", err)
	}

	applySQLFile(t, ctx, conn, "migrations/notification/000003_notification_log_created_at_index.up.sql")
	if !notificationLogIndexExists(t, ctx, conn) {
		t.Fatal("created_at index was not created")
	}
	var definition string
	if err := conn.QueryRow(ctx, `
		SELECT indexdef FROM pg_indexes
		WHERE schemaname = 'notification' AND tablename = 'notification_logs'
		  AND indexname = 'notification_logs_created_at_idx'`).Scan(&definition); err != nil {
		t.Fatalf("read created_at index definition: %v", err)
	}
	if !strings.Contains(definition, "(created_at)") {
		t.Fatalf("index definition = %q, want created_at", definition)
	}
	var retained int
	if err := conn.QueryRow(ctx, `SELECT count(*) FROM notification.notification_logs`).Scan(&retained); err != nil {
		t.Fatalf("count retained logs: %v", err)
	}
	if retained != 1 {
		t.Fatalf("existing notification logs = %d, want 1", retained)
	}

	applySQLFile(t, ctx, conn, "migrations/notification/000003_notification_log_created_at_index.down.sql")
	if notificationLogIndexExists(t, ctx, conn) {
		t.Fatal("created_at index still exists after rollback")
	}
	if err := conn.QueryRow(ctx, `SELECT count(*) FROM notification.notification_logs`).Scan(&retained); err != nil {
		t.Fatalf("count logs after rollback: %v", err)
	}
	if retained != 1 {
		t.Fatalf("rollback removed notification logs: %d", retained)
	}

	applySQLFile(t, ctx, conn, "migrations/notification/000003_notification_log_created_at_index.up.sql")
	if !notificationLogIndexExists(t, ctx, conn) {
		t.Fatal("created_at index was not restored after reapply")
	}
}

func notificationLogIndexExists(t *testing.T, ctx context.Context, conn *pgx.Conn) bool {
	t.Helper()
	var exists bool
	if err := conn.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM pg_indexes
			WHERE schemaname = 'notification' AND tablename = 'notification_logs'
			  AND indexname = 'notification_logs_created_at_idx'
		)`).Scan(&exists); err != nil {
		t.Fatalf("read created_at index: %v", err)
	}
	return exists
}
