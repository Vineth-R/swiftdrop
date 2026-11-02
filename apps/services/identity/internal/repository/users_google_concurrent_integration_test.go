//go:build integration

package repository

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
)

func TestRepository_CreateGoogleUser_concurrentFirstLogin(t *testing.T) {
	tests := []struct {
		name string
		sub  func(int) string
	}{
		{name: "same email different subjects", sub: func(i int) string { return fmt.Sprintf("race-sub-%d", i) }},
		{name: "same email same subject", sub: func(int) string { return "race-sub-shared" }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo, pool := newTestRepository(t)
			ctx := context.Background()
			const email = "race@example.com"

			const callers = 8
			var wg sync.WaitGroup
			results := make(chan error, callers)
			for i := 0; i < callers; i++ {
				wg.Add(1)
				go func(i int) {
					defer wg.Done()
					_, err := repo.CreateGoogleUser(ctx, CreateGoogleUserParams{
						Email: email, FullName: "Race User", GoogleSub: test.sub(i), Role: auth.RoleCustomer,
					})
					results <- err
				}(i)
			}
			wg.Wait()
			close(results)

			succeeded := 0
			for err := range results {
				switch {
				case err == nil:
					succeeded++
				case errors.Is(err, ErrEmailTaken), errors.Is(err, ErrGoogleAccountConflict):
					// Expected for the losing requests.
				default:
					t.Fatalf("unexpected error: %v", err)
				}
			}
			if succeeded != 1 {
				t.Fatalf("%d requests created an account, want exactly 1", succeeded)
			}

			var count int
			if err := pool.QueryRow(ctx,
				`SELECT count(*) FROM identity.users WHERE lower(email) = lower($1)`, email,
			).Scan(&count); err != nil {
				t.Fatalf("count users: %v", err)
			}
			if count != 1 {
				t.Fatalf("%d accounts exist for the email, want 1", count)
			}
		})
	}
}
