//go:build integration

package repository

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Vineth-R/swiftdrop/apps/services/identity/internal/lockout"
	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
)

var integrationNow = time.Date(2026, time.November, 2, 9, 30, 0, 0, time.UTC)

func createTestUser(t *testing.T, repo *Repository, email string) User {
	t.Helper()
	user, err := repo.CreateUser(context.Background(), CreateUserParams{
		Email:        email,
		PasswordHash: "$2a$04$abcdefghijklmnopqrstuuABCDEFGHIJKLMNOPQRSTUVWXYZ0123",
		FullName:     "Nimal Perera",
		Phone:        "+94771234567",
		Role:         auth.RoleCustomer,
	})
	if err != nil {
		t.Fatalf("CreateUser returned error: %v", err)
	}
	return user
}

func TestRepository_CreateUser(t *testing.T) {
	repo, _ := newTestRepository(t)
	ctx := context.Background()

	created := createTestUser(t, repo, "nimal@example.com")
	if created.ID == "" || created.Role != auth.RoleCustomer || !created.IsActive || created.AuthProvider != "LOCAL" {
		t.Fatalf("created user = %+v", created)
	}

	t.Run("is readable by email in any letter case", func(t *testing.T) {
		got, err := repo.UserByEmail(ctx, "NIMAL@Example.COM")
		if err != nil {
			t.Fatalf("UserByEmail returned error: %v", err)
		}
		if got.ID != created.ID || got.Email != "nimal@example.com" || got.Role != auth.RoleCustomer {
			t.Fatalf("UserByEmail = %+v, want the created user", got)
		}
		if got.PasswordHash == nil || got.Phone == nil || *got.Phone != "+94771234567" {
			t.Fatalf("UserByEmail lost stored fields: %+v", got)
		}
	})

	t.Run("is readable by id", func(t *testing.T) {
		got, err := repo.UserByID(ctx, created.ID)
		if err != nil {
			t.Fatalf("UserByID returned error: %v", err)
		}
		if got.Email != "nimal@example.com" {
			t.Fatalf("UserByID email = %q", got.Email)
		}
	})

	t.Run("a duplicate email in another letter case is refused", func(t *testing.T) {
		_, err := repo.CreateUser(ctx, CreateUserParams{
			Email:        "Nimal@Example.com",
			PasswordHash: "hash",
			FullName:     "Someone Else",
			Phone:        "+94770000000",
			Role:         auth.RoleCustomer,
		})
		if !errors.Is(err, ErrEmailTaken) {
			t.Fatalf("CreateUser error = %v, want ErrEmailTaken", err)
		}
	})

	t.Run("an unknown role is refused", func(t *testing.T) {
		_, err := repo.CreateUser(ctx, CreateUserParams{
			Email:        "root@example.com",
			PasswordHash: "hash",
			FullName:     "Root User",
			Phone:        "+94770000001",
			Role:         auth.Role("ROOT"),
		})
		if err == nil || errors.Is(err, ErrEmailTaken) {
			t.Fatalf("CreateUser error = %v, want a role error", err)
		}
	})

	t.Run("a missing user is not found", func(t *testing.T) {
		if _, err := repo.UserByEmail(ctx, "nobody@example.com"); !errors.Is(err, ErrNotFound) {
			t.Fatalf("UserByEmail error = %v, want ErrNotFound", err)
		}
		if _, err := repo.UserByID(ctx, "00000000-0000-4000-8000-000000000000"); !errors.Is(err, ErrNotFound) {
			t.Fatalf("UserByID error = %v, want ErrNotFound", err)
		}
	})
}

func TestRepository_RecordFailedLogin(t *testing.T) {
	repo, _ := newTestRepository(t)
	ctx := context.Background()
	policy := lockout.Policy{MaxFailures: 3, Duration: 15 * time.Minute}
	user := createTestUser(t, repo, "lock@example.com")

	t.Run("failures below the limit do not lock", func(t *testing.T) {
		for attempt := 1; attempt < policy.MaxFailures; attempt++ {
			state, err := repo.RecordFailedLogin(ctx, user.ID, policy, integrationNow)
			if err != nil {
				t.Fatalf("RecordFailedLogin returned error: %v", err)
			}
			if state.FailedCount != attempt || state.LockedUntil != nil {
				t.Fatalf("after attempt %d state = %+v", attempt, state)
			}
		}
	})

	t.Run("the failure that reaches the limit locks and is persisted", func(t *testing.T) {
		state, err := repo.RecordFailedLogin(ctx, user.ID, policy, integrationNow)
		if err != nil {
			t.Fatalf("RecordFailedLogin returned error: %v", err)
		}
		if state.LockedUntil == nil {
			t.Fatalf("state = %+v, want a lock", state)
		}

		stored, err := repo.UserByID(ctx, user.ID)
		if err != nil {
			t.Fatalf("UserByID returned error: %v", err)
		}
		if !stored.LockoutState().Locked(integrationNow.Add(time.Minute)) {
			t.Fatalf("stored state %+v is not locked", stored.LockoutState())
		}
		if stored.FailedLoginCount != policy.MaxFailures {
			t.Fatalf("stored failed count = %d, want %d", stored.FailedLoginCount, policy.MaxFailures)
		}
	})

	t.Run("a failure during the lock does not extend it", func(t *testing.T) {
		before, _ := repo.UserByID(ctx, user.ID)
		state, err := repo.RecordFailedLogin(ctx, user.ID, policy, integrationNow.Add(time.Minute))
		if err != nil {
			t.Fatalf("RecordFailedLogin returned error: %v", err)
		}
		if !state.LockedUntil.Equal(*before.LockedUntil) {
			t.Fatalf("lock moved from %v to %v", before.LockedUntil, state.LockedUntil)
		}
	})

	t.Run("after the lock expires counting restarts", func(t *testing.T) {
		state, err := repo.RecordFailedLogin(ctx, user.ID, policy, integrationNow.Add(16*time.Minute))
		if err != nil {
			t.Fatalf("RecordFailedLogin returned error: %v", err)
		}
		if state.FailedCount != 1 || state.LockedUntil != nil {
			t.Fatalf("state = %+v, want count 1 and no lock", state)
		}
	})

	t.Run("a successful login clears the counter and stamps the time", func(t *testing.T) {
		if err := repo.RecordSuccessfulLogin(ctx, user.ID, integrationNow); err != nil {
			t.Fatalf("RecordSuccessfulLogin returned error: %v", err)
		}
		stored, err := repo.UserByID(ctx, user.ID)
		if err != nil {
			t.Fatalf("UserByID returned error: %v", err)
		}
		if stored.FailedLoginCount != 0 || stored.LockedUntil != nil {
			t.Fatalf("stored state = %+v, want cleared", stored.LockoutState())
		}
	})

	t.Run("an unknown user is not found", func(t *testing.T) {
		const missing = "00000000-0000-4000-8000-000000000000"
		if _, err := repo.RecordFailedLogin(ctx, missing, policy, integrationNow); !errors.Is(err, ErrNotFound) {
			t.Fatalf("RecordFailedLogin error = %v, want ErrNotFound", err)
		}
		if err := repo.RecordSuccessfulLogin(ctx, missing, integrationNow); !errors.Is(err, ErrNotFound) {
			t.Fatalf("RecordSuccessfulLogin error = %v, want ErrNotFound", err)
		}
	})
}

func TestRepository_RecordFailedLogin_concurrent(t *testing.T) {
	repo, _ := newTestRepository(t)
	ctx := context.Background()
	user := createTestUser(t, repo, "race@example.com")
	policy := lockout.Policy{MaxFailures: 100, Duration: time.Minute}

	const attempts = 12
	var wg sync.WaitGroup
	errs := make(chan error, attempts)
	for range attempts {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := repo.RecordFailedLogin(ctx, user.ID, policy, integrationNow)
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("RecordFailedLogin returned error: %v", err)
		}
	}

	stored, err := repo.UserByID(ctx, user.ID)
	if err != nil {
		t.Fatalf("UserByID returned error: %v", err)
	}
	if stored.FailedLoginCount != attempts {
		t.Fatalf("failed count = %d, want %d: concurrent failures were lost", stored.FailedLoginCount, attempts)
	}
}
