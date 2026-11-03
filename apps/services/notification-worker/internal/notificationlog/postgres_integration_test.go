//go:build integration

package notificationlog

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

	"github.com/Vineth-R/swiftdrop/apps/services/notification-worker/internal/idempotency"
)

const notificationLogTestTimeout = 120 * time.Second

// newNotificationLogTestDatabase creates a uniquely named database, applies the
// actual notification migrations, and drops only that database on cleanup.
func newNotificationLogTestDatabase(t *testing.T) (context.Context, *pgx.Conn) {
	t.Helper()

	serverURL := os.Getenv("TEST_DATABASE_URL")
	if serverURL == "" {
		t.Skip("TEST_DATABASE_URL is not set; skipping PostgreSQL notification-log integration tests")
	}

	ctx, cancel := context.WithTimeout(context.Background(), notificationLogTestTimeout)
	t.Cleanup(cancel)

	admin, err := pgx.Connect(ctx, serverURL)
	if err != nil {
		t.Fatalf("connect to test database server: %v", err)
	}

	databaseName := "swiftdrop_notification_log_test_" + notificationLogRandomHex(8)
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

	testURL, err := url.Parse(serverURL)
	if err != nil {
		t.Fatalf("parse TEST_DATABASE_URL: %v", err)
	}
	testURL.Path = "/" + databaseName
	conn, err := pgx.Connect(ctx, testURL.String())
	if err != nil {
		t.Fatalf("connect to isolated database: %v", err)
	}
	t.Cleanup(func() {
		_ = conn.Close(context.Background())
	})

	for _, path := range []string{
		"infra/docker/postgres/init/01-schemas.sql",
		"migrations/notification/000001_initial_schema.up.sql",
		"migrations/notification/000002_tables.up.sql",
	} {
		applyNotificationLogSQLFile(t, ctx, conn, path)
	}
	return ctx, conn
}

func applyNotificationLogSQLFile(t *testing.T, ctx context.Context, conn *pgx.Conn, relativePath string) {
	t.Helper()
	root := filepath.Join("..", "..", "..", "..", "..")
	content, err := os.ReadFile(filepath.Join(root, relativePath))
	if err != nil {
		t.Fatalf("read %s: %v", relativePath, err)
	}
	if _, err := conn.PgConn().Exec(ctx, string(content)).ReadAll(); err != nil {
		t.Fatalf("apply %s: %v", relativePath, err)
	}
}

func notificationLogRandomHex(size int) string {
	buffer := make([]byte, size)
	if _, err := rand.Read(buffer); err != nil {
		panic(fmt.Sprintf("read random bytes: %v", err))
	}
	return hex.EncodeToString(buffer)
}

type notificationLogFixture struct {
	id        string
	status    string
	createdAt time.Time
}

func insertNotificationLog(
	t *testing.T,
	ctx context.Context,
	conn *pgx.Conn,
	status, channel, recipient string,
	createdAt time.Time,
	subject, errorMessage *string,
	sentAt *time.Time,
) notificationLogFixture {
	t.Helper()

	var id string
	if err := conn.QueryRow(ctx, `
		INSERT INTO notification.notification_logs
			(event_id, parcel_id, channel, recipient, subject, status, error_message, created_at, sent_at)
		VALUES (gen_random_uuid(), gen_random_uuid(), $1, $2, $3, $4, $5, $6, $7)
		RETURNING id::text`,
		channel, recipient, subject, status, errorMessage, createdAt, sentAt,
	).Scan(&id); err != nil {
		t.Fatalf("insert notification log: %v", err)
	}
	return notificationLogFixture{id: id, status: status, createdAt: createdAt}
}

func insertNotificationLogWithID(
	t *testing.T,
	ctx context.Context,
	conn *pgx.Conn,
	id, status, channel, recipient string,
	createdAt time.Time,
) notificationLogFixture {
	t.Helper()

	var stored string
	if err := conn.QueryRow(ctx, `
		INSERT INTO notification.notification_logs
			(id, event_id, parcel_id, channel, recipient, status, created_at)
		VALUES ($1::uuid, gen_random_uuid(), gen_random_uuid(), $2, $3, $4, $5)
		RETURNING id::text`,
		id, channel, recipient, status, createdAt,
	).Scan(&stored); err != nil {
		t.Fatalf("insert notification log with explicit id: %v", err)
	}
	return notificationLogFixture{id: stored, status: status, createdAt: createdAt}
}

func seedNotificationLogs(t *testing.T, ctx context.Context, conn *pgx.Conn) map[string]notificationLogFixture {
	t.Helper()

	base := time.Date(2026, time.November, 2, 10, 0, 0, 0, time.UTC)
	subject := "Your parcel has shipped"
	sentAt := base.Add(4 * time.Minute)
	errorMessage := "smtp connection refused"

	fixtures := map[string]notificationLogFixture{}
	fixtures["sent-newest"] = insertNotificationLog(t, ctx, conn,
		idempotency.StatusSent, "EMAIL", "newest@example.com", base.Add(5*time.Minute),
		&subject, nil, &sentAt)
	fixtures["failed-sms"] = insertNotificationLog(t, ctx, conn,
		idempotency.StatusFailed, "SMS", "+94771234567", base.Add(4*time.Minute),
		nil, &errorMessage, nil)
	fixtures["pending"] = insertNotificationLog(t, ctx, conn,
		idempotency.StatusPending, "EMAIL", "pending@example.com", base.Add(3*time.Minute),
		nil, nil, nil)
	fixtures["sent-older"] = insertNotificationLog(t, ctx, conn,
		idempotency.StatusSent, "EMAIL", "older@example.com", base.Add(2*time.Minute),
		&subject, nil, &sentAt)
	fixtures["failed-email"] = insertNotificationLog(t, ctx, conn,
		idempotency.StatusFailed, "EMAIL", "failed@example.com", base.Add(time.Minute),
		nil, &errorMessage, nil)
	return fixtures
}

func TestRepositoryListAllNewestFirst(t *testing.T) {
	ctx, conn := newNotificationLogTestDatabase(t)
	fixtures := seedNotificationLogs(t, ctx, conn)
	repo := NewRepository(conn)

	logs, total, err := repo.List(ctx, Filter{Limit: 20})
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if total != len(fixtures) || len(logs) != len(fixtures) {
		t.Fatalf("total=%d len=%d, want %d", total, len(logs), len(fixtures))
	}
	want := []string{
		fixtures["sent-newest"].id,
		fixtures["failed-sms"].id,
		fixtures["pending"].id,
		fixtures["sent-older"].id,
		fixtures["failed-email"].id,
	}
	for index, id := range want {
		if logs[index].ID != id {
			t.Fatalf("logs[%d].ID = %s, want %s", index, logs[index].ID, id)
		}
	}
	if logs[2].Subject != nil || logs[2].ErrorMessage != nil || logs[2].SentAt != nil {
		t.Fatalf("nullable fields were not preserved: %+v", logs[2])
	}
	if logs[0].Subject == nil || logs[0].SentAt == nil {
		t.Fatalf("expected subject and sent_at on newest sent row: %+v", logs[0])
	}
}

func TestRepositoryListUsesIDTieBreaker(t *testing.T) {
	ctx, conn := newNotificationLogTestDatabase(t)
	repo := NewRepository(conn)
	base := time.Date(2026, time.November, 2, 12, 0, 0, 0, time.UTC)
	lowID := "00000000-0000-4000-8000-000000000001"
	highID := "00000000-0000-4000-8000-000000000002"

	insertNotificationLogWithID(t, ctx, conn, lowID, idempotency.StatusSent, "EMAIL", "tie-low@example.com", base)
	insertNotificationLogWithID(t, ctx, conn, highID, idempotency.StatusSent, "EMAIL", "tie-high@example.com", base)

	logs, total, err := repo.List(ctx, Filter{Limit: 20})
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if total != 2 || len(logs) != 2 {
		t.Fatalf("total=%d len=%d, want 2", total, len(logs))
	}
	if logs[0].ID != highID || logs[1].ID != lowID {
		t.Fatalf("tie order = %s, %s, want %s, %s", logs[0].ID, logs[1].ID, highID, lowID)
	}
}

func TestRepositoryListFiltersByStatus(t *testing.T) {
	ctx, conn := newNotificationLogTestDatabase(t)
	seedNotificationLogs(t, ctx, conn)
	repo := NewRepository(conn)

	for _, status := range []string{idempotency.StatusPending, idempotency.StatusSent, idempotency.StatusFailed} {
		t.Run(status, func(t *testing.T) {
			logs, total, err := repo.List(ctx, Filter{Status: status, Limit: 20})
			if err != nil {
				t.Fatalf("List returned error: %v", err)
			}
			if total != len(logs) || total == 0 {
				t.Fatalf("total=%d len=%d, want matching non-empty rows", total, len(logs))
			}
			for _, entry := range logs {
				if entry.Status != status {
					t.Fatalf("status = %q, want %q", entry.Status, status)
				}
			}
		})
	}
}

func TestRepositoryListPagination(t *testing.T) {
	ctx, conn := newNotificationLogTestDatabase(t)
	fixtures := seedNotificationLogs(t, ctx, conn)
	repo := NewRepository(conn)

	logs, total, err := repo.List(ctx, Filter{Limit: 2, Offset: 2})
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if total != len(fixtures) || len(logs) != 2 {
		t.Fatalf("total=%d len=%d, want %d and 2", total, len(logs), len(fixtures))
	}
	if logs[0].ID != fixtures["pending"].id || logs[1].ID != fixtures["sent-older"].id {
		t.Fatalf("page rows = %s, %s", logs[0].ID, logs[1].ID)
	}

	logs, total, err = repo.List(ctx, Filter{Limit: 2, Offset: 99})
	if err != nil {
		t.Fatalf("List beyond the end returned error: %v", err)
	}
	if total != len(fixtures) || logs == nil || len(logs) != 0 {
		t.Fatalf("beyond-end list = %+v total=%d", logs, total)
	}
}

func TestRepositoryListEmptyTable(t *testing.T) {
	ctx, conn := newNotificationLogTestDatabase(t)
	repo := NewRepository(conn)

	logs, total, err := repo.List(ctx, Filter{Limit: 20})
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if total != 0 || logs == nil || len(logs) != 0 {
		t.Fatalf("empty list = %+v total=%d", logs, total)
	}
}
