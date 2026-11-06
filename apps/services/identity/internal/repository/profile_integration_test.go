//go:build integration

package repository

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

const (
	originalPasswordHash = "$2a$04$abcdefghijklmnopqrstuuABCDEFGHIJKLMNOPQRSTUVWXYZ0123" // what createTestUser stores
	changedPasswordHash  = "$2a$04$ChangedChangedChangedCOABCDEFGHIJKLMNOPQRSTUVWXYZ0123"
)

func text(value string) *string { return &value }

func TestRepository_UpdateProfile(t *testing.T) {
	repo, pool := newTestRepository(t)
	ctx := context.Background()
	user := createTestUser(t, repo, "profile@example.com")
	bystander := createTestUser(t, repo, "bystander@example.com")

	t.Run("changes only the fields that are set", func(t *testing.T) {
		got, err := repo.UpdateProfile(ctx, user.ID, ProfileChange{FullName: text("Nimal P. Perera")})
		if err != nil {
			t.Fatalf("UpdateProfile returned error: %v", err)
		}
		if got.FullName != "Nimal P. Perera" || got.Phone == nil || *got.Phone != "+94771234567" {
			t.Fatalf("name %q phone %v, want the new name and the old phone", got.FullName, got.Phone)
		}

		got, err = repo.UpdateProfile(ctx, user.ID, ProfileChange{Phone: text("+94771234000")})
		if err != nil {
			t.Fatalf("UpdateProfile returned error: %v", err)
		}
		if got.FullName != "Nimal P. Perera" || got.Phone == nil || *got.Phone != "+94771234000" {
			t.Fatalf("name %q phone %v, want the earlier name and the new phone", got.FullName, got.Phone)
		}
	})

	t.Run("returns the stored account and leaves everything else alone", func(t *testing.T) {
		got, err := repo.UpdateProfile(ctx, user.ID, ProfileChange{FullName: text("Nimali Perera"), Phone: text("+94711112222")})
		if err != nil {
			t.Fatalf("UpdateProfile returned error: %v", err)
		}
		reloaded, err := repo.UserByID(ctx, user.ID)
		if err != nil {
			t.Fatalf("UserByID returned error: %v", err)
		}
		if got.FullName != reloaded.FullName || *got.Phone != *reloaded.Phone {
			t.Fatalf("returned %+v, stored %+v", got, reloaded)
		}
		if reloaded.Email != "profile@example.com" || reloaded.Role != user.Role || reloaded.AuthProvider != "LOCAL" ||
			!reloaded.IsActive || reloaded.PasswordHash == nil || *reloaded.PasswordHash != originalPasswordHash {
			t.Fatalf("an unrelated field changed: %+v", reloaded)
		}
	})

	t.Run("an empty change is harmless", func(t *testing.T) {
		before, _ := repo.UserByID(ctx, user.ID)
		got, err := repo.UpdateProfile(ctx, user.ID, ProfileChange{})
		if err != nil {
			t.Fatalf("UpdateProfile returned error: %v", err)
		}
		if got.FullName != before.FullName || *got.Phone != *before.Phone {
			t.Fatalf("an empty change altered the account: %+v", got)
		}
	})

	t.Run("does not touch another user", func(t *testing.T) {
		other, err := repo.UserByID(ctx, bystander.ID)
		if err != nil {
			t.Fatalf("UserByID returned error: %v", err)
		}
		if other.FullName != "Nimal Perera" || other.Phone == nil || *other.Phone != "+94771234567" {
			t.Fatalf("another user changed: %+v", other)
		}
	})

	t.Run("stamps updated_at", func(t *testing.T) {
		var before, after time.Time
		if err := pool.QueryRow(ctx, `SELECT updated_at FROM identity.users WHERE id = $1`, user.ID).Scan(&before); err != nil {
			t.Fatalf("read updated_at: %v", err)
		}
		if _, err := repo.UpdateProfile(ctx, user.ID, ProfileChange{FullName: text("Nimal Perera")}); err != nil {
			t.Fatalf("UpdateProfile returned error: %v", err)
		}
		if err := pool.QueryRow(ctx, `SELECT updated_at FROM identity.users WHERE id = $1`, user.ID).Scan(&after); err != nil {
			t.Fatalf("read updated_at: %v", err)
		}
		if !after.After(before) {
			t.Fatalf("updated_at = %v, want later than %v", after, before)
		}
	})

	t.Run("reports an unknown user", func(t *testing.T) {
		_, err := repo.UpdateProfile(ctx, "00000000-0000-0000-0000-000000000000", ProfileChange{FullName: text("Nobody")})
		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("error = %v, want ErrNotFound", err)
		}
	})
}

func activeTokens(t *testing.T, db DB, userID string) int {
	t.Helper()
	var count int
	if err := db.QueryRow(context.Background(),
		`SELECT count(*) FROM identity.refresh_tokens WHERE user_id = $1 AND revoked_at IS NULL`, userID,
	).Scan(&count); err != nil {
		t.Fatalf("count active tokens: %v", err)
	}
	return count
}

func TestRepository_ChangePassword(t *testing.T) {
	ctx := context.Background()
	expiry := integrationNow.Add(7 * 24 * time.Hour)
	changeTime := integrationNow.Add(time.Minute)

	t.Run("replaces the hash, clears the lock and ends only this user's sessions", func(t *testing.T) {
		repo, pool := newTestRepository(t)
		user := createTestUser(t, repo, "change@example.com")
		other := createTestUser(t, repo, "bystander@example.com")
		for _, raw := range []string{"rt_a", "rt_b"} {
			if _, err := repo.CreateRefreshToken(ctx, newToken(user.ID, raw, expiry)); err != nil {
				t.Fatalf("create refresh token: %v", err)
			}
		}
		if _, err := repo.CreateRefreshToken(ctx, newToken(other.ID, "rt_other", expiry)); err != nil {
			t.Fatalf("create refresh token: %v", err)
		}
		if _, err := pool.Exec(ctx,
			`UPDATE identity.users SET failed_login_count = 5, locked_until = $2 WHERE id = $1`, user.ID, integrationNow.Add(time.Hour),
		); err != nil {
			t.Fatalf("lock user: %v", err)
		}

		if err := repo.ChangePassword(ctx, user.ID, originalPasswordHash, changedPasswordHash, changeTime); err != nil {
			t.Fatalf("ChangePassword returned error: %v", err)
		}

		reloaded, err := repo.UserByID(ctx, user.ID)
		if err != nil {
			t.Fatalf("UserByID returned error: %v", err)
		}
		if reloaded.PasswordHash == nil || *reloaded.PasswordHash != changedPasswordHash {
			t.Fatal("password hash was not replaced")
		}
		if reloaded.FailedLoginCount != 0 || reloaded.LockedUntil != nil {
			t.Fatalf("lockout was not cleared: %+v", reloaded.LockoutState())
		}
		if got := activeTokens(t, pool, user.ID); got != 0 {
			t.Fatalf("%d refresh tokens still active after the change", got)
		}
		if got := activeTokens(t, pool, other.ID); got != 1 {
			t.Fatal("another user's session was revoked")
		}
		if _, err := repo.RotateRefreshToken(ctx, sha256Hex("rt_a"), NextRefreshToken{TokenHash: sha256Hex("rt_next"), ExpiresAt: expiry}, changeTime); !errors.Is(err, ErrRefreshTokenInvalid) {
			t.Fatalf("rotating a revoked token: error = %v, want ErrRefreshTokenInvalid", err)
		}
	})

	t.Run("keeps the revocation time of a session that already ended", func(t *testing.T) {
		repo, pool := newTestRepository(t)
		user := createTestUser(t, repo, "ended@example.com")
		if _, err := repo.CreateRefreshToken(ctx, newToken(user.ID, "rt_old", expiry)); err != nil {
			t.Fatalf("create refresh token: %v", err)
		}
		if err := repo.RevokeRefreshToken(ctx, user.ID, sha256Hex("rt_old"), integrationNow); err != nil {
			t.Fatalf("RevokeRefreshToken returned error: %v", err)
		}

		if err := repo.ChangePassword(ctx, user.ID, originalPasswordHash, changedPasswordHash, changeTime); err != nil {
			t.Fatalf("ChangePassword returned error: %v", err)
		}

		var revokedAt time.Time
		if err := pool.QueryRow(ctx, `SELECT revoked_at FROM identity.refresh_tokens WHERE token_hash = $1`, sha256Hex("rt_old")).Scan(&revokedAt); err != nil {
			t.Fatalf("read revoked_at: %v", err)
		}
		if !revokedAt.Equal(integrationNow) {
			t.Fatalf("revoked_at = %v, want the original %v", revokedAt, integrationNow)
		}
	})

	t.Run("refuses a hash that is no longer the stored one and changes nothing", func(t *testing.T) {
		repo, pool := newTestRepository(t)
		user := createTestUser(t, repo, "stale@example.com")
		if _, err := repo.CreateRefreshToken(ctx, newToken(user.ID, "rt_keep", expiry)); err != nil {
			t.Fatalf("create refresh token: %v", err)
		}

		err := repo.ChangePassword(ctx, user.ID, "$2a$04$SomeOtherHashThatIsNotTheStoredOne0123456789ABCDEFGHIJKLMN", changedPasswordHash, changeTime)

		if !errors.Is(err, ErrPasswordMismatch) {
			t.Fatalf("error = %v, want ErrPasswordMismatch", err)
		}
		reloaded, _ := repo.UserByID(ctx, user.ID)
		if *reloaded.PasswordHash != originalPasswordHash {
			t.Fatal("a refused change replaced the hash")
		}
		if got := activeTokens(t, pool, user.ID); got != 1 {
			t.Fatalf("%d active tokens, want the session kept", got)
		}
	})

	t.Run("an account without a password cannot be changed", func(t *testing.T) {
		repo, pool := newTestRepository(t)
		user := createTestUser(t, repo, "google@example.com")
		if _, err := pool.Exec(ctx,
			`UPDATE identity.users SET password_hash = NULL, google_sub = 'google-sub-1', auth_provider = 'GOOGLE' WHERE id = $1`, user.ID,
		); err != nil {
			t.Fatalf("make the account Google-only: %v", err)
		}

		err := repo.ChangePassword(ctx, user.ID, originalPasswordHash, changedPasswordHash, changeTime)

		if !errors.Is(err, ErrPasswordMismatch) {
			t.Fatalf("error = %v, want ErrPasswordMismatch", err)
		}
		if reloaded, _ := repo.UserByID(ctx, user.ID); reloaded.PasswordHash != nil {
			t.Fatal("a Google-only account was given a password")
		}
	})

	t.Run("reports an unknown user", func(t *testing.T) {
		repo, _ := newTestRepository(t)

		err := repo.ChangePassword(ctx, "00000000-0000-0000-0000-000000000000", originalPasswordHash, changedPasswordHash, changeTime)

		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("error = %v, want ErrNotFound", err)
		}
	})

	t.Run("two changes that both know the old password cannot both win", func(t *testing.T) {
		repo, _ := newTestRepository(t)
		user := createTestUser(t, repo, "race@example.com")

		const callers = 6
		var wg sync.WaitGroup
		results := make(chan error, callers)
		for i := 0; i < callers; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				hash := changedPasswordHash[:len(changedPasswordHash)-1] + string(rune('A'+i))
				results <- repo.ChangePassword(ctx, user.ID, originalPasswordHash, hash, changeTime)
			}(i)
		}
		wg.Wait()
		close(results)

		wins, mismatches := 0, 0
		for err := range results {
			switch {
			case err == nil:
				wins++
			case errors.Is(err, ErrPasswordMismatch):
				mismatches++
			default:
				t.Fatalf("unexpected error: %v", err)
			}
		}
		if wins != 1 || mismatches != callers-1 {
			t.Fatalf("%d changes succeeded and %d were refused, want 1 and %d", wins, mismatches, callers-1)
		}
	})
}
