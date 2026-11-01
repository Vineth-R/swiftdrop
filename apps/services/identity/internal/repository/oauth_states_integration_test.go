//go:build integration

package repository

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestRepository_ConsumeOAuthState_worksOnce(t *testing.T) {
	repo, _ := newTestRepository(t)
	ctx := context.Background()
	now := time.Now()

	if err := repo.CreateOAuthState(ctx, NewOAuthState{
		State:        "state-once",
		CodeVerifier: "verifier-1",
		RedirectTo:   "/dashboard",
		ExpiresAt:    now.Add(10 * time.Minute),
	}); err != nil {
		t.Fatalf("CreateOAuthState returned error: %v", err)
	}

	got, err := repo.ConsumeOAuthState(ctx, "state-once", now)
	if err != nil {
		t.Fatalf("first ConsumeOAuthState returned error: %v", err)
	}
	if got.CodeVerifier != "verifier-1" || got.RedirectTo != "/dashboard" {
		t.Fatalf("ConsumeOAuthState() = %+v", got)
	}

	if _, err := repo.ConsumeOAuthState(ctx, "state-once", now); !errors.Is(err, ErrOAuthStateInvalid) {
		t.Fatalf("replayed state error = %v, want ErrOAuthStateInvalid", err)
	}
}

func TestRepository_ConsumeOAuthState_rejectsUnknownAndExpired(t *testing.T) {
	repo, _ := newTestRepository(t)
	ctx := context.Background()
	now := time.Now()

	if err := repo.CreateOAuthState(ctx, NewOAuthState{State: "state-old", ExpiresAt: now.Add(-time.Second)}); err != nil {
		t.Fatalf("CreateOAuthState returned error: %v", err)
	}

	for _, state := range []string{"state-old", "never-issued"} {
		if _, err := repo.ConsumeOAuthState(ctx, state, now); !errors.Is(err, ErrOAuthStateInvalid) {
			t.Fatalf("ConsumeOAuthState(%q) error = %v, want ErrOAuthStateInvalid", state, err)
		}
	}
}

func TestRepository_ConsumeOAuthState_optionalFieldsAreEmpty(t *testing.T) {
	repo, _ := newTestRepository(t)
	ctx := context.Background()
	now := time.Now()

	if err := repo.CreateOAuthState(ctx, NewOAuthState{State: "state-bare", ExpiresAt: now.Add(time.Minute)}); err != nil {
		t.Fatalf("CreateOAuthState returned error: %v", err)
	}
	got, err := repo.ConsumeOAuthState(ctx, "state-bare", now)
	if err != nil {
		t.Fatalf("ConsumeOAuthState returned error: %v", err)
	}
	if got != (OAuthState{}) {
		t.Fatalf("ConsumeOAuthState() = %+v, want empty fields", got)
	}
}

func TestRepository_CreateOAuthState_refusesAbsoluteRedirect(t *testing.T) {
	repo, _ := newTestRepository(t)
	ctx := context.Background()

	for _, redirect := range []string{"https://evil.example", "//evil.example"} {
		err := repo.CreateOAuthState(ctx, NewOAuthState{
			State: "state-redirect", RedirectTo: redirect, ExpiresAt: time.Now().Add(time.Minute),
		})
		if err == nil {
			t.Fatalf("CreateOAuthState accepted redirect %q", redirect)
		}
	}
}

func TestRepository_ConsumeOAuthState_concurrentCallbacksOnlyOneWins(t *testing.T) {
	repo, _ := newTestRepository(t)
	ctx := context.Background()
	now := time.Now()

	if err := repo.CreateOAuthState(ctx, NewOAuthState{State: "state-race", ExpiresAt: now.Add(time.Minute)}); err != nil {
		t.Fatalf("CreateOAuthState returned error: %v", err)
	}

	const callers = 8
	var wg sync.WaitGroup
	results := make(chan error, callers)
	for range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := repo.ConsumeOAuthState(ctx, "state-race", now)
			results <- err
		}()
	}
	wg.Wait()
	close(results)

	succeeded := 0
	for err := range results {
		if err == nil {
			succeeded++
		} else if !errors.Is(err, ErrOAuthStateInvalid) {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if succeeded != 1 {
		t.Fatalf("%d callbacks consumed the same state, want exactly 1", succeeded)
	}
}
