//go:build integration

package repository

import (
	"context"
	"errors"
	"net/netip"
	"strings"
	"sync"
	"testing"
	"time"
)

func newToken(userID, raw string, expiresAt time.Time) NewRefreshToken {
	return NewRefreshToken{UserID: userID, TokenHash: sha256Hex(raw), ExpiresAt: expiresAt}
}

func nextToken(raw string, expiresAt time.Time) NextRefreshToken {
	return NextRefreshToken{TokenHash: sha256Hex(raw), ExpiresAt: expiresAt}
}

func TestRepository_CreateRefreshToken(t *testing.T) {
	repo, pool := newTestRepository(t)
	ctx := context.Background()
	user := createTestUser(t, repo, "tokens@example.com")

	address := netip.MustParseAddr("203.0.113.9")
	id, err := repo.CreateRefreshToken(ctx, NewRefreshToken{
		UserID:    user.ID,
		TokenHash: sha256Hex("rt_first"),
		ExpiresAt: integrationNow.Add(7 * 24 * time.Hour),
		UserAgent: strings.Repeat("x", 400) + "\xff",
		IPAddress: &address,
	})
	if err != nil {
		t.Fatalf("CreateRefreshToken returned error: %v", err)
	}

	var userAgentLength int
	var storedAddress string
	if err := pool.QueryRow(ctx,
		`SELECT length(user_agent), host(ip_address) FROM identity.refresh_tokens WHERE id = $1`, id,
	).Scan(&userAgentLength, &storedAddress); err != nil {
		t.Fatalf("read stored token: %v", err)
	}
	if userAgentLength != userAgentMaxLength || storedAddress != "203.0.113.9" {
		t.Fatalf("stored user agent length %d, address %q", userAgentLength, storedAddress)
	}

	if _, err := repo.CreateRefreshToken(ctx, newToken(user.ID, "rt_first", integrationNow.Add(time.Hour))); err == nil {
		t.Fatal("CreateRefreshToken accepted a duplicate token hash")
	}
}

func TestRepository_RotateRefreshToken(t *testing.T) {
	repo, pool := newTestRepository(t)
	ctx := context.Background()
	user := createTestUser(t, repo, "rotate@example.com")
	expiry := integrationNow.Add(7 * 24 * time.Hour)

	if _, err := repo.CreateRefreshToken(ctx, newToken(user.ID, "rt_one", expiry)); err != nil {
		t.Fatalf("CreateRefreshToken returned error: %v", err)
	}

	t.Run("rotation revokes the old token and links the new one", func(t *testing.T) {
		userID, err := repo.RotateRefreshToken(ctx, sha256Hex("rt_one"), nextToken("rt_two", expiry), integrationNow)
		if err != nil {
			t.Fatalf("RotateRefreshToken returned error: %v", err)
		}
		if userID != user.ID {
			t.Fatalf("rotated user = %q, want %q", userID, user.ID)
		}

		var revoked bool
		var linked bool
		if err := pool.QueryRow(ctx, `
			SELECT old.revoked_at IS NOT NULL, old.replaced_by = new.id
			FROM identity.refresh_tokens old, identity.refresh_tokens new
			WHERE old.token_hash = $1 AND new.token_hash = $2`,
			sha256Hex("rt_one"), sha256Hex("rt_two"),
		).Scan(&revoked, &linked); err != nil {
			t.Fatalf("read rotation chain: %v", err)
		}
		if !revoked || !linked {
			t.Fatalf("old token revoked = %t, linked to the new one = %t", revoked, linked)
		}
	})

	t.Run("a used token cannot be used again", func(t *testing.T) {
		_, err := repo.RotateRefreshToken(ctx, sha256Hex("rt_one"), nextToken("rt_three", expiry), integrationNow)
		if !errors.Is(err, ErrRefreshTokenInvalid) {
			t.Fatalf("RotateRefreshToken error = %v, want ErrRefreshTokenInvalid", err)
		}
		var stored int
		_ = pool.QueryRow(ctx, `SELECT count(*) FROM identity.refresh_tokens WHERE token_hash = $1`, sha256Hex("rt_three")).Scan(&stored)
		if stored != 0 {
			t.Fatal("a refused rotation still stored a replacement token")
		}
	})

	t.Run("the replacement token works", func(t *testing.T) {
		if _, err := repo.RotateRefreshToken(ctx, sha256Hex("rt_two"), nextToken("rt_three", expiry), integrationNow); err != nil {
			t.Fatalf("RotateRefreshToken returned error: %v", err)
		}
	})

	t.Run("an unknown token is invalid", func(t *testing.T) {
		_, err := repo.RotateRefreshToken(ctx, sha256Hex("rt_unknown"), nextToken("rt_x", expiry), integrationNow)
		if !errors.Is(err, ErrRefreshTokenInvalid) {
			t.Fatalf("RotateRefreshToken error = %v, want ErrRefreshTokenInvalid", err)
		}
	})
}

func TestRepository_RotateRefreshToken_expiredAndDeactivated(t *testing.T) {
	repo, pool := newTestRepository(t)
	ctx := context.Background()
	user := createTestUser(t, repo, "stale@example.com")

	if _, err := repo.CreateRefreshToken(ctx, newToken(user.ID, "rt_old", integrationNow.Add(time.Hour))); err != nil {
		t.Fatalf("CreateRefreshToken returned error: %v", err)
	}
	if _, err := repo.CreateRefreshToken(ctx, newToken(user.ID, "rt_live", integrationNow.Add(48*time.Hour))); err != nil {
		t.Fatalf("CreateRefreshToken returned error: %v", err)
	}

	t.Run("an expired token is invalid", func(t *testing.T) {
		_, err := repo.RotateRefreshToken(ctx, sha256Hex("rt_old"), nextToken("rt_n1", integrationNow.Add(72*time.Hour)), integrationNow.Add(2*time.Hour))
		if !errors.Is(err, ErrRefreshTokenInvalid) {
			t.Fatalf("RotateRefreshToken error = %v, want ErrRefreshTokenInvalid", err)
		}
	})

	t.Run("a deactivated user's token is invalid", func(t *testing.T) {
		if _, err := pool.Exec(ctx, `UPDATE identity.users SET is_active = false WHERE id = $1`, user.ID); err != nil {
			t.Fatalf("deactivate user: %v", err)
		}
		_, err := repo.RotateRefreshToken(ctx, sha256Hex("rt_live"), nextToken("rt_n2", integrationNow.Add(72*time.Hour)), integrationNow)
		if !errors.Is(err, ErrRefreshTokenInvalid) {
			t.Fatalf("RotateRefreshToken error = %v, want ErrRefreshTokenInvalid", err)
		}
	})
}

func TestRepository_RotateRefreshToken_concurrent(t *testing.T) {
	repo, _ := newTestRepository(t)
	ctx := context.Background()
	user := createTestUser(t, repo, "double@example.com")
	expiry := integrationNow.Add(7 * 24 * time.Hour)

	if _, err := repo.CreateRefreshToken(ctx, newToken(user.ID, "rt_shared", expiry)); err != nil {
		t.Fatalf("CreateRefreshToken returned error: %v", err)
	}

	const attempts = 6
	var wg sync.WaitGroup
	results := make(chan error, attempts)
	for i := range attempts {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := repo.RotateRefreshToken(
				ctx,
				sha256Hex("rt_shared"),
				nextToken("rt_replacement_"+string(rune('a'+i)), expiry),
				integrationNow,
			)
			results <- err
		}()
	}
	wg.Wait()
	close(results)

	succeeded := 0
	for err := range results {
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, ErrRefreshTokenInvalid):
		default:
			t.Fatalf("RotateRefreshToken returned an unexpected error: %v", err)
		}
	}
	if succeeded != 1 {
		t.Fatalf("%d concurrent rotations of one token succeeded, want exactly 1", succeeded)
	}
}

func TestRepository_RevokeRefreshToken(t *testing.T) {
	repo, _ := newTestRepository(t)
	ctx := context.Background()
	owner := createTestUser(t, repo, "owner@example.com")
	other := createTestUser(t, repo, "other@example.com")
	expiry := integrationNow.Add(7 * 24 * time.Hour)

	if _, err := repo.CreateRefreshToken(ctx, newToken(owner.ID, "rt_logout", expiry)); err != nil {
		t.Fatalf("CreateRefreshToken returned error: %v", err)
	}

	t.Run("another user cannot revoke the token", func(t *testing.T) {
		if err := repo.RevokeRefreshToken(ctx, other.ID, sha256Hex("rt_logout"), integrationNow); err != nil {
			t.Fatalf("RevokeRefreshToken returned error: %v", err)
		}
		if _, err := repo.RotateRefreshToken(ctx, sha256Hex("rt_logout"), nextToken("rt_after", expiry), integrationNow); err != nil {
			t.Fatalf("the token was revoked by a different user: %v", err)
		}
	})

	t.Run("the owner revokes it and it stops working", func(t *testing.T) {
		if err := repo.RevokeRefreshToken(ctx, owner.ID, sha256Hex("rt_after"), integrationNow); err != nil {
			t.Fatalf("RevokeRefreshToken returned error: %v", err)
		}
		_, err := repo.RotateRefreshToken(ctx, sha256Hex("rt_after"), nextToken("rt_again", expiry), integrationNow)
		if !errors.Is(err, ErrRefreshTokenInvalid) {
			t.Fatalf("RotateRefreshToken error = %v, want ErrRefreshTokenInvalid", err)
		}
	})

	t.Run("revoking is idempotent", func(t *testing.T) {
		if err := repo.RevokeRefreshToken(ctx, owner.ID, sha256Hex("rt_after"), integrationNow); err != nil {
			t.Fatalf("second RevokeRefreshToken returned error: %v", err)
		}
		if err := repo.RevokeRefreshToken(ctx, owner.ID, sha256Hex("rt_never_existed"), integrationNow); err != nil {
			t.Fatalf("RevokeRefreshToken of an unknown token returned error: %v", err)
		}
	})
}

func TestRepository_RotateRefreshToken_reuse(t *testing.T) {
	repo, pool := newTestRepository(t)
	ctx := context.Background()
	user := createTestUser(t, repo, "reuse@example.com")
	expiry := integrationNow.Add(7 * 24 * time.Hour)

	create := func(t *testing.T, raw string) {
		t.Helper()
		if _, err := repo.CreateRefreshToken(ctx, newToken(user.ID, raw, expiry)); err != nil {
			t.Fatalf("CreateRefreshToken(%s) returned error: %v", raw, err)
		}
	}
	rotate := func(t *testing.T, from, to string, at time.Time) {
		t.Helper()
		if _, err := repo.RotateRefreshToken(ctx, sha256Hex(from), nextToken(to, expiry), at); err != nil {
			t.Fatalf("RotateRefreshToken(%s -> %s) returned error: %v", from, to, err)
		}
	}
	active := func(t *testing.T, raws ...string) int {
		t.Helper()
		hashes := make([]string, len(raws))
		for i, raw := range raws {
			hashes[i] = sha256Hex(raw)
		}
		var count int
		if err := pool.QueryRow(ctx,
			`SELECT count(*) FROM identity.refresh_tokens WHERE token_hash = ANY($1) AND revoked_at IS NULL`, hashes,
		).Scan(&count); err != nil {
			t.Fatalf("count active tokens: %v", err)
		}
		return count
	}

	t.Run("an old token replayed after the grace period revokes its whole chain", func(t *testing.T) {
		create(t, "rt_a1")
		create(t, "rt_other_device")
		rotate(t, "rt_a1", "rt_a2", integrationNow)
		rotate(t, "rt_a2", "rt_a3", integrationNow.Add(time.Minute))

		replay := integrationNow.Add(2 * time.Minute)
		userID, err := repo.RotateRefreshToken(ctx, sha256Hex("rt_a1"), nextToken("rt_thief", expiry), replay)
		if !errors.Is(err, ErrRefreshTokenReused) || !errors.Is(err, ErrRefreshTokenInvalid) {
			t.Fatalf("replay error = %v, want ErrRefreshTokenReused (an ErrRefreshTokenInvalid)", err)
		}
		if userID != user.ID {
			t.Fatalf("replay reported user %q, want %q", userID, user.ID)
		}

		if got := active(t, "rt_a1", "rt_a2", "rt_a3"); got != 0 {
			t.Fatalf("%d tokens of the replayed chain are still active, want 0", got)
		}
		if got := active(t, "rt_thief"); got != 0 {
			t.Fatal("the refused replay stored a replacement token")
		}
		_, err = repo.RotateRefreshToken(ctx, sha256Hex("rt_a3"), nextToken("rt_a4", expiry), replay)
		if !errors.Is(err, ErrRefreshTokenInvalid) {
			t.Fatalf("the newest token of the chain still rotates: error = %v", err)
		}
	})

	t.Run("another login of the same user is left alone", func(t *testing.T) {
		if got := active(t, "rt_other_device"); got != 1 {
			t.Fatalf("the other device's token was revoked (active = %d)", got)
		}
		rotate(t, "rt_other_device", "rt_other_device_2", integrationNow.Add(3*time.Minute))
	})

	t.Run("a retry inside the grace period is refused but revokes nothing", func(t *testing.T) {
		create(t, "rt_c1")
		rotate(t, "rt_c1", "rt_c2", integrationNow)

		_, err := repo.RotateRefreshToken(ctx, sha256Hex("rt_c1"), nextToken("rt_c3", expiry), integrationNow.Add(RefreshReuseGrace-time.Second))
		if !errors.Is(err, ErrRefreshTokenInvalid) || errors.Is(err, ErrRefreshTokenReused) {
			t.Fatalf("retry error = %v, want a plain ErrRefreshTokenInvalid", err)
		}
		if got := active(t, "rt_c2"); got != 1 {
			t.Fatal("a retry inside the grace period revoked the newest token")
		}
		rotate(t, "rt_c2", "rt_c3", integrationNow.Add(RefreshReuseGrace))
	})

	t.Run("the grace period ends exactly at its limit", func(t *testing.T) {
		create(t, "rt_e1")
		rotate(t, "rt_e1", "rt_e2", integrationNow)

		_, err := repo.RotateRefreshToken(ctx, sha256Hex("rt_e1"), nextToken("rt_e3", expiry), integrationNow.Add(RefreshReuseGrace))
		if !errors.Is(err, ErrRefreshTokenReused) {
			t.Fatalf("replay at the limit: error = %v, want ErrRefreshTokenReused", err)
		}
	})

	t.Run("a token revoked by logout is not a reuse", func(t *testing.T) {
		create(t, "rt_d1")
		create(t, "rt_d_other")
		if err := repo.RevokeRefreshToken(ctx, user.ID, sha256Hex("rt_d1"), integrationNow); err != nil {
			t.Fatalf("RevokeRefreshToken returned error: %v", err)
		}

		_, err := repo.RotateRefreshToken(ctx, sha256Hex("rt_d1"), nextToken("rt_d2", expiry), integrationNow.Add(time.Hour))
		if !errors.Is(err, ErrRefreshTokenInvalid) || errors.Is(err, ErrRefreshTokenReused) {
			t.Fatalf("error = %v, want a plain ErrRefreshTokenInvalid", err)
		}
		if got := active(t, "rt_d_other"); got != 1 {
			t.Fatal("replaying a logged out token revoked another login")
		}
	})

	t.Run("a replay of a chain that has already ended is still reported", func(t *testing.T) {
		create(t, "rt_f1")
		rotate(t, "rt_f1", "rt_f2", integrationNow)
		if err := repo.RevokeRefreshToken(ctx, user.ID, sha256Hex("rt_f2"), integrationNow.Add(time.Minute)); err != nil {
			t.Fatalf("RevokeRefreshToken returned error: %v", err)
		}

		_, err := repo.RotateRefreshToken(ctx, sha256Hex("rt_f1"), nextToken("rt_f3", expiry), integrationNow.Add(time.Hour))
		if !errors.Is(err, ErrRefreshTokenReused) {
			t.Fatalf("error = %v, want ErrRefreshTokenReused", err)
		}
	})
}
