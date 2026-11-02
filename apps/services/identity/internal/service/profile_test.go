package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
)

func TestService_Profile(t *testing.T) {
	f := newFixture(t)
	account := f.register(t)

	t.Run("returns the account of the signed-in user", func(t *testing.T) {
		got, err := f.service.Profile(context.Background(), account.ID)
		if err != nil {
			t.Fatalf("Profile returned error: %v", err)
		}

		want := Profile{
			ID:           account.ID,
			Email:        "nimal@example.com",
			FullName:     "Nimal Perera",
			Phone:        "+94771234567",
			Role:         auth.RoleCustomer,
			AuthProvider: "LOCAL",
			CreatedAt:    time.Date(2026, time.November, 2, 9, 0, 0, 0, time.UTC),
		}
		if got != want {
			t.Fatalf("Profile = %+v, want %+v", got, want)
		}
	})

	t.Run("an account without a phone has an empty phone", func(t *testing.T) {
		f.store.users[account.ID].Phone = nil

		got, err := f.service.Profile(context.Background(), account.ID)
		if err != nil {
			t.Fatalf("Profile returned error: %v", err)
		}
		if got.Phone != "" {
			t.Fatalf("Phone = %q, want empty", got.Phone)
		}
	})

	t.Run("reports the sign-in provider", func(t *testing.T) {
		f.store.users[account.ID].AuthProvider = "GOOGLE"

		got, err := f.service.Profile(context.Background(), account.ID)
		if err != nil {
			t.Fatalf("Profile returned error: %v", err)
		}
		if got.AuthProvider != "GOOGLE" {
			t.Fatalf("AuthProvider = %q, want GOOGLE", got.AuthProvider)
		}
	})
}

func TestService_Profile_refusesAccountsThatAreGoneOrDisabled(t *testing.T) {
	f := newFixture(t)
	account := f.register(t)

	t.Run("an unknown user is not found", func(t *testing.T) {
		if _, err := f.service.Profile(context.Background(), "no-such-user"); !errors.Is(err, ErrUserNotFound) {
			t.Fatalf("Profile error = %v, want ErrUserNotFound", err)
		}
	})

	t.Run("a deactivated account is refused even with a valid token", func(t *testing.T) {
		f.store.users[account.ID].IsActive = false

		_, err := f.service.Profile(context.Background(), account.ID)
		if !errors.Is(err, ErrAccountDisabled) {
			t.Fatalf("Profile error = %v, want ErrAccountDisabled", err)
		}
	})
}

func TestService_Profile_storeFailure(t *testing.T) {
	f := newFixture(t)
	account := f.register(t)
	f.store.failOnOps["UserByID"] = true

	_, err := f.service.Profile(context.Background(), account.ID)
	if !errors.Is(err, errStore) {
		t.Fatalf("Profile error = %v, want the store error wrapped", err)
	}
	if errors.Is(err, ErrUserNotFound) || errors.Is(err, ErrAccountDisabled) {
		t.Fatalf("a store failure must not look like a missing or disabled account: %v", err)
	}
}
