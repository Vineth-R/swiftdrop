//go:build integration

package repository

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRepository_ConsumeOAuthState_expiryBoundary(t *testing.T) {
	repo, _ := newTestRepository(t)
	ctx := context.Background()
	now := time.Unix(1_800_000_000, 0).UTC()
	binding := testBindingHash()

	if err := repo.CreateOAuthState(ctx, NewOAuthState{
		State: "state-exactly-now", BrowserBindingHash: binding, ExpiresAt: now,
	}); err != nil {
		t.Fatalf("CreateOAuthState returned error: %v", err)
	}
	if _, err := repo.ConsumeOAuthState(ctx, "state-exactly-now", binding, now); !errors.Is(err, ErrOAuthStateInvalid) {
		t.Fatalf("state expiring exactly now error = %v, want ErrOAuthStateInvalid", err)
	}

	if err := repo.CreateOAuthState(ctx, NewOAuthState{
		State: "state-just-live", BrowserBindingHash: binding, ExpiresAt: now.Add(time.Microsecond),
	}); err != nil {
		t.Fatalf("CreateOAuthState returned error: %v", err)
	}
	if _, err := repo.ConsumeOAuthState(ctx, "state-just-live", binding, now); err != nil {
		t.Fatalf("state expiring one microsecond later should be valid (PostgreSQL timestamptz precision): %v", err)
	}
}
