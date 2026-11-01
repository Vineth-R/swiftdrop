//go:build integration

package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
)

func createGoogleTestUser(t *testing.T, repo *Repository, email, sub string) User {
	t.Helper()
	user, err := repo.CreateGoogleUser(context.Background(), CreateGoogleUserParams{
		Email: email, FullName: "Google Person", GoogleSub: sub, Role: auth.RoleCustomer,
	})
	if err != nil {
		t.Fatalf("CreateGoogleUser returned error: %v", err)
	}
	return user
}

func TestRepository_CreateGoogleUser(t *testing.T) {
	repo, pool := newTestRepository(t)
	ctx := context.Background()

	user := createGoogleTestUser(t, repo, "google@example.com", "sub-1")

	var (
		provider      string
		hasPassword   bool
		emailVerified bool
		sub           string
		role          string
	)
	if err := pool.QueryRow(ctx, `
		SELECT u.auth_provider, u.password_hash IS NOT NULL, u.email_verified, u.google_sub, r.name
		FROM identity.users u JOIN identity.roles r ON r.id = u.role_id
		WHERE u.id = $1`, user.ID,
	).Scan(&provider, &hasPassword, &emailVerified, &sub, &role); err != nil {
		t.Fatalf("read stored user: %v", err)
	}
	if provider != "GOOGLE" || hasPassword || !emailVerified || sub != "sub-1" || role != "CUSTOMER" {
		t.Fatalf("stored user: provider=%s hasPassword=%t emailVerified=%t sub=%s role=%s",
			provider, hasPassword, emailVerified, sub, role)
	}

	t.Run("found by google_sub", func(t *testing.T) {
		found, err := repo.UserByGoogleSub(ctx, "sub-1")
		if err != nil || found.ID != user.ID || found.PasswordHash != nil {
			t.Fatalf("UserByGoogleSub() = %+v, %v", found, err)
		}
		if _, err := repo.UserByGoogleSub(ctx, "unknown"); !errors.Is(err, ErrNotFound) {
			t.Fatalf("unknown sub error = %v, want ErrNotFound", err)
		}
	})

	t.Run("email taken in any letter case", func(t *testing.T) {
		_, err := repo.CreateGoogleUser(ctx, CreateGoogleUserParams{
			Email: "GOOGLE@example.com", FullName: "Someone Else", GoogleSub: "sub-2", Role: auth.RoleCustomer,
		})
		if !errors.Is(err, ErrEmailTaken) {
			t.Fatalf("error = %v, want ErrEmailTaken", err)
		}
	})

	t.Run("google_sub already linked", func(t *testing.T) {
		_, err := repo.CreateGoogleUser(ctx, CreateGoogleUserParams{
			Email: "other@example.com", FullName: "Someone Else", GoogleSub: "sub-1", Role: auth.RoleCustomer,
		})
		if !errors.Is(err, ErrGoogleAccountConflict) {
			t.Fatalf("error = %v, want ErrGoogleAccountConflict", err)
		}
	})

	t.Run("unknown role", func(t *testing.T) {
		_, err := repo.CreateGoogleUser(ctx, CreateGoogleUserParams{
			Email: "role@example.com", FullName: "Someone Else", GoogleSub: "sub-3", Role: auth.Role("NOPE"),
		})
		if err == nil {
			t.Fatal("CreateGoogleUser accepted an unknown role")
		}
	})
}

func TestRepository_LinkGoogleAccount(t *testing.T) {
	repo, pool := newTestRepository(t)
	ctx := context.Background()
	local := createTestUser(t, repo, "local@example.com")
	if _, err := repo.CreateRefreshToken(ctx, newToken(local.ID, "rt_before_link", time.Now().Add(time.Hour))); err != nil {
		t.Fatalf("CreateRefreshToken returned error: %v", err)
	}

	cleared, err := repo.LinkGoogleAccount(ctx, local.ID, "sub-local", time.Now())
	if err != nil {
		t.Fatalf("LinkGoogleAccount returned error: %v", err)
	}
	if !cleared {
		t.Fatal("an unverified account's password was not cleared")
	}

	var provider string
	var hasPassword, verified bool
	var revoked bool
	if err := pool.QueryRow(ctx, `
		SELECT u.auth_provider, u.password_hash IS NOT NULL, u.email_verified,
		       EXISTS (
		           SELECT 1 FROM identity.refresh_tokens t
		           WHERE t.user_id = u.id AND t.revoked_at IS NOT NULL
		       )
		FROM identity.users u WHERE u.id = $1`, local.ID,
	).Scan(&provider, &hasPassword, &verified, &revoked); err != nil {
		t.Fatalf("read user: %v", err)
	}
	if provider != "LOCAL" || hasPassword || !verified || !revoked {
		t.Fatalf("after linking: provider=%s hasPassword=%t verified=%t revoked=%t, want LOCAL, false, true, true",
			provider, hasPassword, verified, revoked)
	}

	t.Run("linking the same account again is harmless", func(t *testing.T) {
		cleared, err := repo.LinkGoogleAccount(ctx, local.ID, "sub-local", time.Now())
		if err != nil || cleared {
			t.Fatalf("second link = cleared %t, error %v; want false, nil", cleared, err)
		}
	})

	t.Run("a different google account cannot replace the link", func(t *testing.T) {
		if _, err := repo.LinkGoogleAccount(ctx, local.ID, "sub-attacker", time.Now()); !errors.Is(err, ErrGoogleAccountConflict) {
			t.Fatalf("error = %v, want ErrGoogleAccountConflict", err)
		}
	})

	t.Run("a google account cannot be linked to two users", func(t *testing.T) {
		second := createTestUser(t, repo, "second@example.com")
		if _, err := repo.LinkGoogleAccount(ctx, second.ID, "sub-local", time.Now()); !errors.Is(err, ErrGoogleAccountConflict) {
			t.Fatalf("error = %v, want ErrGoogleAccountConflict", err)
		}
	})

	t.Run("unknown user", func(t *testing.T) {
		if _, err := repo.LinkGoogleAccount(ctx, "00000000-0000-0000-0000-000000000000", "sub-x", time.Now()); !errors.Is(err, ErrNotFound) {
			t.Fatalf("error = %v, want ErrNotFound", err)
		}
	})
}

func TestRepository_LinkGoogleAccount_preservesVerifiedPassword(t *testing.T) {
	repo, pool := newTestRepository(t)
	ctx := context.Background()
	local := createTestUser(t, repo, "verified@example.com")
	if _, err := pool.Exec(ctx, `UPDATE identity.users SET email_verified = true WHERE id = $1`, local.ID); err != nil {
		t.Fatalf("mark email verified: %v", err)
	}

	cleared, err := repo.LinkGoogleAccount(ctx, local.ID, "sub-verified", time.Now())
	if err != nil {
		t.Fatalf("LinkGoogleAccount returned error: %v", err)
	}
	if cleared {
		t.Fatal("a verified account's password was cleared")
	}

	var hasPassword bool
	if err := pool.QueryRow(ctx, `SELECT password_hash IS NOT NULL FROM identity.users WHERE id = $1`, local.ID).Scan(&hasPassword); err != nil {
		t.Fatalf("read password state: %v", err)
	}
	if !hasPassword {
		t.Fatal("a verified account lost its password")
	}
}

func TestRepository_LinkGoogleAccount_thenPasswordResetRestoresPassword(t *testing.T) {
	repo, pool := newTestRepository(t)
	ctx := context.Background()
	local := createTestUser(t, repo, "reset-after-link@example.com")

	if _, err := repo.LinkGoogleAccount(ctx, local.ID, "sub-reset", time.Now()); err != nil {
		t.Fatalf("LinkGoogleAccount returned error: %v", err)
	}

	const rawToken = "reset-token-after-google-link"
	tokenHash := sha256Hex(rawToken)
	now := time.Now()
	if err := repo.IssuePasswordResetToken(ctx, NewPasswordResetToken{
		UserID: local.ID, TokenHash: tokenHash, ExpiresAt: now.Add(30 * time.Minute),
	}, now); err != nil {
		t.Fatalf("IssuePasswordResetToken returned error: %v", err)
	}

	const newHash = "$2a$04$newhashaftergoogllinknewhashaftergoogllinknewhashaftergo"
	if _, err := repo.ResetPasswordWithToken(ctx, tokenHash, newHash, now.Add(time.Minute)); err != nil {
		t.Fatalf("ResetPasswordWithToken returned error: %v", err)
	}

	var storedHash string
	var verified bool
	if err := pool.QueryRow(ctx,
		`SELECT password_hash, email_verified FROM identity.users WHERE id = $1`, local.ID,
	).Scan(&storedHash, &verified); err != nil {
		t.Fatalf("read reset user: %v", err)
	}
	if storedHash != newHash || !verified {
		t.Fatalf("after reset: hash=%q verified=%t, want the new hash and verified email", storedHash, verified)
	}
}

func TestRepository_PurgeExpiredOAuthStates(t *testing.T) {
	repo, _ := newTestRepository(t)
	ctx := context.Background()
	now := time.Now()

	binding := testBindingHash()
	for state, expires := range map[string]time.Time{"old": now.Add(-time.Minute), "live": now.Add(time.Minute)} {
		if err := repo.CreateOAuthState(ctx, NewOAuthState{
			State: state, BrowserBindingHash: binding, ExpiresAt: expires,
		}); err != nil {
			t.Fatalf("CreateOAuthState(%s) returned error: %v", state, err)
		}
	}
	if err := repo.PurgeExpiredOAuthStates(ctx, now); err != nil {
		t.Fatalf("PurgeExpiredOAuthStates returned error: %v", err)
	}

	if _, err := repo.ConsumeOAuthState(ctx, "live", binding, now); err != nil {
		t.Fatalf("live state was purged: %v", err)
	}
	if _, err := repo.ConsumeOAuthState(ctx, "old", binding, now.Add(-2*time.Minute)); !errors.Is(err, ErrOAuthStateInvalid) {
		t.Fatalf("expired state survived the purge: %v", err)
	}
}
