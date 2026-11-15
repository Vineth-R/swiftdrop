//go:build integration

package repository

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
)

func TestCleanupExpiredTokensKeepsActiveAndRecentRows(t *testing.T) {
	repo, pool := newTestRepository(t)
	ctx := context.Background()
	createUser := func(name string) string {
		t.Helper()
		user, err := repo.CreateUser(ctx, CreateUserParams{
			Email: name + "@example.test", PasswordHash: "$2a$10$fixture",
			FullName: name, Phone: "+94770000000", Role: auth.RoleCustomer,
		})
		if err != nil {
			t.Fatalf("CreateUser(%s): %v", name, err)
		}
		return user.ID
	}
	refreshUser := createUser("cleanup-refresh")
	resetExpiredUser := createUser("cleanup-reset-expired")
	resetRecentUser := createUser("cleanup-reset-recent")
	verifyExpiredUser := createUser("cleanup-verify-expired")
	verifyRecentUser := createUser("cleanup-verify-recent")

	now := time.Now().UTC()
	mustExec := func(query string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, query, args...); err != nil {
			t.Fatalf("fixture SQL: %v", err)
		}
	}
	mustExec(`INSERT INTO identity.refresh_tokens (user_id, token_hash, expires_at) VALUES ($1,$2,$3)`,
		refreshUser, strings.Repeat("a", 64), now.Add(-2*time.Hour))
	mustExec(`INSERT INTO identity.refresh_tokens (user_id, token_hash, expires_at) VALUES ($1,$2,$3)`,
		refreshUser, strings.Repeat("b", 64), now.Add(-30*time.Minute))

	mustExec(`INSERT INTO identity.password_reset_tokens (user_id, token_hash, expires_at) VALUES ($1,$2,$3)`,
		resetExpiredUser, strings.Repeat("c", 64), now.Add(-2*time.Hour))
	mustExec(`INSERT INTO identity.password_reset_tokens (user_id, token_hash, expires_at) VALUES ($1,$2,$3)`,
		resetRecentUser, strings.Repeat("d", 64), now.Add(-30*time.Minute))
	mustExec(`INSERT INTO identity.password_reset_tokens (user_id, token_hash, expires_at, used_at) VALUES ($1,$2,$3,$4)`,
		resetRecentUser, strings.Repeat("e", 64), now.Add(time.Hour), now.Add(-2*time.Hour))
	mustExec(`INSERT INTO identity.password_reset_tokens (user_id, token_hash, expires_at, used_at) VALUES ($1,$2,$3,$4)`,
		resetRecentUser, strings.Repeat("f", 64), now.Add(time.Hour), now.Add(-30*time.Minute))

	mustExec(`INSERT INTO identity.email_verification_tokens (user_id, token_hash, expires_at) VALUES ($1,$2,$3)`,
		verifyExpiredUser, strings.Repeat("g", 64), now.Add(-2*time.Hour))
	mustExec(`INSERT INTO identity.email_verification_tokens (user_id, token_hash, expires_at) VALUES ($1,$2,$3)`,
		verifyRecentUser, strings.Repeat("h", 64), now.Add(-30*time.Minute))

	deleted, err := repo.CleanupExpiredTokens(ctx, now.Add(-time.Hour), 100)
	if err != nil {
		t.Fatalf("CleanupExpiredTokens: %v", err)
	}
	if deleted != 4 {
		t.Fatalf("deleted = %d, want 4", deleted)
	}
	for table, want := range map[string]int{
		"refresh_tokens":            1,
		"password_reset_tokens":     2,
		"email_verification_tokens": 1,
	} {
		var remaining int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM identity.`+table).Scan(&remaining); err != nil {
			t.Fatal(err)
		}
		if remaining != want {
			t.Fatalf("%s rows = %d, want %d", table, remaining, want)
		}
	}

	if _, err := repo.CleanupExpiredTokens(ctx, now, 100); err != nil {
		t.Fatalf("second cleanup: %v", err)
	}
}
