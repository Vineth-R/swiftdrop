//go:build integration

package repository

import (
	"context"
	"testing"
	"time"
)

func TestRepository_ListAuditLogs(t *testing.T) {
	repo, _ := newTestRepository(t)
	ctx := context.Background()
	admin := createTestUser(t, repo, "audit-admin@example.com")
	created := createCourier(t, repo, admin.ID, "audit-courier@example.com")

	rows, total, err := repo.ListAuditLogs(ctx, AuditLogFilter{ActorUserID: admin.ID, Limit: 10, Offset: 0})
	if err != nil {
		t.Fatalf("ListAuditLogs returned error: %v", err)
	}
	if total != 1 || len(rows) != 1 {
		t.Fatalf("rows = %d, total = %d, want one audit row", len(rows), total)
	}
	if rows[0].Action != "USER_CREATED" || rows[0].EntityID == nil || *rows[0].EntityID != created.ID {
		t.Fatalf("audit row = %+v", rows[0])
	}

	rows, total, err = repo.ListAuditLogs(ctx, AuditLogFilter{Action: "USER_CREATED", Limit: 10, Offset: 0})
	if err != nil || total != 1 || len(rows) != 1 {
		t.Fatalf("action filter: rows=%d total=%d err=%v", len(rows), total, err)
	}
	rows, total, err = repo.ListAuditLogs(ctx, AuditLogFilter{From: timePtr(time.Now().Add(time.Hour)), Limit: 10})
	if err != nil || total != 0 || len(rows) != 0 {
		t.Fatalf("future range: rows=%d total=%d err=%v", len(rows), total, err)
	}
}

func timePtr(value time.Time) *time.Time { return &value }
