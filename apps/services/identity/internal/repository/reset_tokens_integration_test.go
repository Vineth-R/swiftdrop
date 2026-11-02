//go:build integration

package repository

import (
	"context"
	"errors"
	"net/netip"
	"sync"
	"testing"
	"time"
)

const newPasswordHash = "$2a$04$ZYXWVUTSRQPONMLKJIHGFeabcdefghijklmnopqrstuvwxyz0123"

func issueToken(t *testing.T, repo *Repository, userID, raw string, expires time.Time) {
	t.Helper()
	if err := repo.IssuePasswordResetToken(context.Background(), NewPasswordResetToken{
		UserID: userID, TokenHash: sha256Hex(raw), ExpiresAt: expires,
	}, integrationNow); err != nil {
		t.Fatalf("IssuePasswordResetToken(%s) returned error: %v", raw, err)
	}
}

func TestRepository_IssuePasswordResetToken_onlyTheNewestWorks(t *testing.T) {
	repo, pool := newTestRepository(t)
	ctx := context.Background()
	user := createTestUser(t, repo, "reset@example.com")
	expiry := integrationNow.Add(30 * time.Minute)

	address := netip.MustParseAddr("203.0.113.9")
	if err := repo.IssuePasswordResetToken(ctx, NewPasswordResetToken{
		UserID: user.ID, TokenHash: sha256Hex("first"), ExpiresAt: expiry, RequestedIP: &address,
	}, integrationNow); err != nil {
		t.Fatalf("first issue returned error: %v", err)
	}
	issueToken(t, repo, user.ID, "second", expiry)

	var unused int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM identity.password_reset_tokens WHERE user_id = $1 AND used_at IS NULL`, user.ID,
	).Scan(&unused); err != nil {
		t.Fatalf("count unused tokens: %v", err)
	}
	if unused != 1 {
		t.Fatalf("%d unused tokens, want exactly 1", unused)
	}

	var storedIP string
	if err := pool.QueryRow(ctx,
		`SELECT host(requested_ip) FROM identity.password_reset_tokens WHERE token_hash = $1`, sha256Hex("first"),
	).Scan(&storedIP); err != nil || storedIP != "203.0.113.9" {
		t.Fatalf("stored requested IP = %q (%v)", storedIP, err)
	}

	if _, err := repo.ResetPasswordWithToken(ctx, sha256Hex("first"), newPasswordHash, integrationNow); !errors.Is(err, ErrResetTokenInvalid) {
		t.Fatalf("old link error = %v, want ErrResetTokenInvalid", err)
	}
	if _, err := repo.ResetPasswordWithToken(ctx, sha256Hex("second"), newPasswordHash, integrationNow); err != nil {
		t.Fatalf("newest link returned error: %v", err)
	}
}

func TestRepository_IssuePasswordResetToken_unknownUser(t *testing.T) {
	repo, _ := newTestRepository(t)

	err := repo.IssuePasswordResetToken(context.Background(), NewPasswordResetToken{
		UserID: "00000000-0000-0000-0000-000000000000", TokenHash: sha256Hex("x"), ExpiresAt: integrationNow.Add(time.Minute),
	}, integrationNow)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("error = %v, want ErrNotFound", err)
	}
}

func TestRepository_IssuePasswordResetToken_concurrentRequestsLeaveOneUnusedToken(t *testing.T) {
	repo, pool := newTestRepository(t)
	ctx := context.Background()
	user := createTestUser(t, repo, "race@example.com")

	const callers = 6
	var wg sync.WaitGroup
	errs := make(chan error, callers)
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs <- repo.IssuePasswordResetToken(ctx, NewPasswordResetToken{
				UserID: user.ID, TokenHash: sha256Hex("race-" + string(rune('a'+i))), ExpiresAt: integrationNow.Add(time.Hour),
			}, integrationNow)
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent issue failed: %v", err)
		}
	}

	var unused int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM identity.password_reset_tokens WHERE user_id = $1 AND used_at IS NULL`, user.ID,
	).Scan(&unused); err != nil || unused != 1 {
		t.Fatalf("unused tokens = %d (%v), want 1", unused, err)
	}
}

func TestRepository_ResetPasswordWithToken_changesPasswordAndEndsSessions(t *testing.T) {
	repo, pool := newTestRepository(t)
	ctx := context.Background()
	user := createTestUser(t, repo, "sessions@example.com")
	expiry := integrationNow.Add(7 * 24 * time.Hour)

	if _, err := repo.CreateRefreshToken(ctx, newToken(user.ID, "rt_a", expiry)); err != nil {
		t.Fatalf("create refresh token: %v", err)
	}
	if _, err := repo.CreateRefreshToken(ctx, newToken(user.ID, "rt_b", expiry)); err != nil {
		t.Fatalf("create refresh token: %v", err)
	}
	other := createTestUser(t, repo, "bystander@example.com")
	if _, err := repo.CreateRefreshToken(ctx, newToken(other.ID, "rt_other", expiry)); err != nil {
		t.Fatalf("create refresh token: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`UPDATE identity.users SET failed_login_count = 5, locked_until = $2 WHERE id = $1`, user.ID, integrationNow.Add(time.Hour),
	); err != nil {
		t.Fatalf("lock user: %v", err)
	}

	issueToken(t, repo, user.ID, "link", integrationNow.Add(30*time.Minute))
	got, err := repo.ResetPasswordWithToken(ctx, sha256Hex("link"), newPasswordHash, integrationNow.Add(time.Minute))
	if err != nil || got != user.ID {
		t.Fatalf("ResetPasswordWithToken() = %q, %v, want %q", got, err, user.ID)
	}

	reloaded, err := repo.UserByID(ctx, user.ID)
	if err != nil {
		t.Fatalf("UserByID returned error: %v", err)
	}
	if reloaded.PasswordHash == nil || *reloaded.PasswordHash != newPasswordHash {
		t.Fatal("password hash was not replaced")
	}
	if reloaded.FailedLoginCount != 0 || reloaded.LockedUntil != nil {
		t.Fatalf("lockout was not cleared: %+v", reloaded.LockoutState())
	}

	var active, otherActive int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM identity.refresh_tokens WHERE user_id = $1 AND revoked_at IS NULL`, user.ID).Scan(&active)
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM identity.refresh_tokens WHERE user_id = $1 AND revoked_at IS NULL`, other.ID).Scan(&otherActive)
	if active != 0 {
		t.Fatalf("%d refresh tokens still active after a reset", active)
	}
	if otherActive != 1 {
		t.Fatal("another user's session was revoked")
	}

	if _, err := repo.ResetPasswordWithToken(ctx, sha256Hex("link"), newPasswordHash, integrationNow.Add(time.Minute)); !errors.Is(err, ErrResetTokenInvalid) {
		t.Fatalf("second use error = %v, want ErrResetTokenInvalid", err)
	}
}

func TestRepository_ResetPasswordWithToken_rejectsBadTokens(t *testing.T) {
	repo, _ := newTestRepository(t)
	ctx := context.Background()
	user := createTestUser(t, repo, "bad@example.com")
	issueToken(t, repo, user.ID, "short-lived", integrationNow.Add(30*time.Minute))

	tests := []struct {
		name string
		hash string
		now  time.Time
	}{
		{name: "unknown", hash: sha256Hex("unknown"), now: integrationNow},
		{name: "expired at the boundary", hash: sha256Hex("short-lived"), now: integrationNow.Add(30 * time.Minute)},
		{name: "expired later", hash: sha256Hex("short-lived"), now: integrationNow.Add(time.Hour)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := repo.ResetPasswordWithToken(ctx, test.hash, newPasswordHash, test.now); !errors.Is(err, ErrResetTokenInvalid) {
				t.Fatalf("error = %v, want ErrResetTokenInvalid", err)
			}
		})
	}

	reloaded, _ := repo.UserByID(ctx, user.ID)
	if reloaded.PasswordHash != nil && *reloaded.PasswordHash == newPasswordHash {
		t.Fatal("a rejected token changed the password")
	}
}

func TestRepository_ResetPasswordWithToken_deactivatedUser(t *testing.T) {
	repo, pool := newTestRepository(t)
	ctx := context.Background()
	user := createTestUser(t, repo, "off@example.com")
	issueToken(t, repo, user.ID, "link", integrationNow.Add(30*time.Minute))
	if _, err := pool.Exec(ctx, `UPDATE identity.users SET is_active = false WHERE id = $1`, user.ID); err != nil {
		t.Fatalf("deactivate user: %v", err)
	}

	if _, err := repo.ResetPasswordWithToken(ctx, sha256Hex("link"), newPasswordHash, integrationNow); !errors.Is(err, ErrResetTokenInvalid) {
		t.Fatalf("error = %v, want ErrResetTokenInvalid", err)
	}
}

func TestRepository_ResetPasswordWithToken_concurrentUseSucceedsOnce(t *testing.T) {
	repo, _ := newTestRepository(t)
	ctx := context.Background()
	user := createTestUser(t, repo, "once@example.com")
	issueToken(t, repo, user.ID, "link", integrationNow.Add(30*time.Minute))

	const callers = 6
	var wg sync.WaitGroup
	results := make(chan error, callers)
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := repo.ResetPasswordWithToken(ctx, sha256Hex("link"), newPasswordHash, integrationNow)
			results <- err
		}()
	}
	wg.Wait()
	close(results)

	succeeded := 0
	for err := range results {
		if err == nil {
			succeeded++
		} else if !errors.Is(err, ErrResetTokenInvalid) {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if succeeded != 1 {
		t.Fatalf("%d requests used the same token, want exactly 1", succeeded)
	}
}

func TestRepository_PasswordResetTokenUsable(t *testing.T) {
	repo, pool := newTestRepository(t)
	ctx := context.Background()
	user := createTestUser(t, repo, "usable@example.com")
	issueToken(t, repo, user.ID, "live", integrationNow.Add(30*time.Minute))

	usable := func(raw string, at time.Time) bool {
		t.Helper()
		ok, err := repo.PasswordResetTokenUsable(ctx, sha256Hex(raw), at)
		if err != nil {
			t.Fatalf("PasswordResetTokenUsable(%s) returned error: %v", raw, err)
		}
		return ok
	}

	if !usable("live", integrationNow) {
		t.Fatal("a fresh token was reported unusable")
	}
	if !usable("live", integrationNow) {
		t.Fatal("checking a token must not spend it")
	}
	if usable("never-issued", integrationNow) {
		t.Fatal("an unknown token was reported usable")
	}
	if usable("live", integrationNow.Add(30*time.Minute)) {
		t.Fatal("a token was usable at its expiry time")
	}

	if _, err := repo.ResetPasswordWithToken(ctx, sha256Hex("live"), newPasswordHash, integrationNow); err != nil {
		t.Fatalf("spend returned error: %v", err)
	}
	if usable("live", integrationNow) {
		t.Fatal("a spent token was reported usable")
	}

	issueToken(t, repo, user.ID, "second", integrationNow.Add(30*time.Minute))
	if _, err := pool.Exec(ctx, `UPDATE identity.users SET is_active = false WHERE id = $1`, user.ID); err != nil {
		t.Fatalf("deactivate user: %v", err)
	}
	if usable("second", integrationNow) {
		t.Fatal("a token of a deactivated user was reported usable")
	}
}
