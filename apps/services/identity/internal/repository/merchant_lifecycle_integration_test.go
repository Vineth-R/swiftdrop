//go:build integration

package repository

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"testing"
	"time"
)

func transitionID() string {
	var value [16]byte
	_, _ = rand.Read(value[:])
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", value[0:4], value[4:6], value[6:8], value[8:10], value[10:16])
}

func TestMerchantLifecycleTransitionsAreIdempotentAndBumpVersion(t *testing.T) {
	repo, pool := newTestRepository(t)
	ctx := context.Background()
	user, created, err := repo.CreateInactiveMerchantOwner(ctx, "owner@example.test", "hash", "Owner", "+94771234567")
	if err != nil || !created {
		t.Fatalf("create merchant owner: created=%v err=%v", created, err)
	}
	if user.IsActive || user.SessionVersion != 0 {
		t.Fatalf("new owner active=%v version=%d", user.IsActive, user.SessionVersion)
	}
	first := transitionID()
	applied, err := repo.ApplyMerchantLifecycleTransition(ctx, user.ID, first, 1, "APPROVE", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if applied.AlreadyApplied || applied.SessionVersion != 1 || !applied.IsActive {
		t.Fatalf("first transition = %+v", applied)
	}
	again, err := repo.ApplyMerchantLifecycleTransition(ctx, user.ID, first, 1, "APPROVE", time.Now())
	if err != nil || !again.AlreadyApplied || again.SessionVersion != 1 {
		t.Fatalf("retry = %+v err=%v", again, err)
	}
	if _, err := repo.ApplyMerchantLifecycleTransition(ctx, user.ID, first, 2, "APPROVE", time.Now()); !errors.Is(err, ErrTransitionConflict) {
		t.Fatalf("mismatched transition error = %v, want ErrTransitionConflict", err)
	}
	other, _, err := repo.CreateInactiveMerchantOwner(ctx, "other-owner@example.test", "hash", "Other Owner", "+94771234568")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ApplyMerchantLifecycleTransition(ctx, other.ID, first, 1, "APPROVE", time.Now()); !errors.Is(err, ErrTransitionConflict) {
		t.Fatalf("transition reused for another user error = %v, want ErrTransitionConflict", err)
	}
	if _, err := repo.ApplyMerchantLifecycleTransition(ctx, user.ID, transitionID(), 1, "SUSPEND", time.Now()); !errors.Is(err, ErrTransitionStale) {
		t.Fatalf("stale sequence error = %v, want ErrTransitionStale", err)
	}

	hash, _ := hashRefreshForTest(t, repo, user.ID)
	suspended, err := repo.ApplyMerchantLifecycleTransition(ctx, user.ID, transitionID(), 2, "SUSPEND", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if suspended.SessionVersion != 2 || suspended.IsActive || suspended.RevokedSessions != 1 {
		t.Fatalf("suspension = %+v", suspended)
	}
	var revoked bool
	if err := pool.QueryRow(ctx, `SELECT revoked_at IS NOT NULL FROM identity.refresh_tokens WHERE token_hash = $1`, hash).Scan(&revoked); err != nil {
		t.Fatal(err)
	}
	if !revoked {
		t.Fatal("refresh token was not revoked")
	}
	reapproved, err := repo.ApplyMerchantLifecycleTransition(ctx, user.ID, transitionID(), 3, "APPROVE", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if reapproved.SessionVersion != 3 || !reapproved.IsActive {
		t.Fatalf("reapproval = %+v", reapproved)
	}
}

func hashRefreshForTest(t *testing.T, repo *Repository, userID string) (string, string) {
	t.Helper()
	hash := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	if _, err := repo.CreateRefreshToken(context.Background(), NewRefreshToken{
		UserID: userID, TokenHash: hash, ExpiresAt: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	return hash, hash
}

// The refusals are named errors so the internal API can answer 409, not 500.
func TestMerchantOwnerRefusalsAreNamedErrors(t *testing.T) {
	repo, pool := newTestRepository(t)
	ctx := context.Background()
	unknown := "3f1c7a52-0000-4000-8000-0000000000ff"

	customer := createTestUser(t, repo, "customer@example.test")
	if _, err := repo.ApplyMerchantLifecycleTransition(ctx, customer.ID, transitionID(), 1, "APPROVE", time.Now()); !errors.Is(err, ErrNotMerchantOwner) {
		t.Fatalf("lifecycle for a customer error = %v, want ErrNotMerchantOwner", err)
	}
	if err := repo.DeletePendingMerchantOwner(ctx, customer.ID); !errors.Is(err, ErrOwnerNotEligible) {
		t.Fatalf("removing a customer error = %v, want ErrOwnerNotEligible", err)
	}

	owner, _, err := repo.CreateInactiveMerchantOwner(ctx, "refusal-owner@example.test", "hash", "Owner", "+94771234567")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ApplyMerchantLifecycleTransition(ctx, owner.ID, transitionID(), 1, "APPROVE", time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := repo.DeletePendingMerchantOwner(ctx, owner.ID); !errors.Is(err, ErrOwnerNotEligible) {
		t.Fatalf("removing an approved owner error = %v, want ErrOwnerNotEligible", err)
	}

	if _, err := repo.ApplyMerchantLifecycleTransition(ctx, unknown, transitionID(), 1, "APPROVE", time.Now()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("lifecycle for an unknown user error = %v, want ErrNotFound", err)
	}
	if err := repo.DeletePendingMerchantOwner(ctx, unknown); !errors.Is(err, ErrNotFound) {
		t.Fatalf("removing an unknown user error = %v, want ErrNotFound", err)
	}
	var stillThere int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM identity.users WHERE id IN ($1, $2)`, customer.ID, owner.ID).Scan(&stillThere); err != nil || stillThere != 2 {
		t.Fatalf("accounts left = %d (%v), want 2: a refused removal must delete nothing", stillThere, err)
	}
}
