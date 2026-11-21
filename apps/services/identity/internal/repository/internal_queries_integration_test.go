//go:build integration

package repository

import (
	"context"
	"errors"
	"testing"

	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
)

func TestRepository_CourierEligibility(t *testing.T) {
	repo, pool := newTestRepository(t)
	ctx := context.Background()

	courier, err := repo.CreateUser(ctx, CreateUserParams{
		Email: "courier@example.com", PasswordHash: "$2a$04$abcdefghijklmnopqrstuuABCDEFGHIJKLMNOPQRSTUVWXYZ0123",
		FullName: "Kasun Silva", Phone: "+94712223344", Role: auth.RoleCourier,
	})
	if err != nil {
		t.Fatalf("create courier: %v", err)
	}
	customer := createTestUser(t, repo, "customer@example.com")

	// A courier with no profile row is not available.
	got, err := repo.CourierEligibility(ctx, courier.ID)
	if err != nil || got != (CourierEligibility{IsActive: true, IsAvailable: false, Role: auth.RoleCourier}) {
		t.Fatalf("courier without profile: %+v, %v", got, err)
	}

	if _, err := pool.Exec(ctx, `INSERT INTO identity.courier_profiles (user_id, is_available) VALUES ($1, true)`, courier.ID); err != nil {
		t.Fatalf("insert profile: %v", err)
	}
	got, err = repo.CourierEligibility(ctx, courier.ID)
	if err != nil || got != (CourierEligibility{IsActive: true, IsAvailable: true, Role: auth.RoleCourier}) {
		t.Fatalf("available courier: %+v, %v", got, err)
	}

	if _, err := pool.Exec(ctx, `UPDATE identity.courier_profiles SET is_available = false WHERE user_id = $1`, courier.ID); err != nil {
		t.Fatalf("update profile: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE identity.users SET is_active = false WHERE id = $1`, courier.ID); err != nil {
		t.Fatalf("deactivate: %v", err)
	}
	got, err = repo.CourierEligibility(ctx, courier.ID)
	if err != nil || got != (CourierEligibility{IsActive: false, IsAvailable: false, Role: auth.RoleCourier}) {
		t.Fatalf("inactive unavailable courier: %+v, %v", got, err)
	}

	got, err = repo.CourierEligibility(ctx, customer.ID)
	if err != nil || got.Role != auth.RoleCustomer || got.IsAvailable {
		t.Fatalf("non-courier: %+v, %v", got, err)
	}

	if _, err := repo.CourierEligibility(ctx, "00000000-0000-0000-0000-000000000000"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown ID error = %v, want ErrNotFound", err)
	}
}

func TestRepository_UserContact(t *testing.T) {
	repo, _ := newTestRepository(t)
	ctx := context.Background()
	local := createTestUser(t, repo, "contact@example.com")
	google := createGoogleTestUser(t, repo, "nophone@example.com", "sub-contact")

	got, err := repo.UserContact(ctx, local.ID)
	if err != nil || got.FullName != "Nimal Perera" || got.Email != "contact@example.com" || got.Phone == nil || *got.Phone != "+94771234567" {
		t.Fatalf("UserContact() = %+v, %v", got, err)
	}

	got, err = repo.UserContact(ctx, google.ID)
	if err != nil || got.Phone != nil || got.Email != "nophone@example.com" {
		t.Fatalf("UserContact() for a Google user = %+v, %v, want a nil phone", got, err)
	}

	if _, err := repo.UserContact(ctx, "00000000-0000-0000-0000-000000000000"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown ID error = %v, want ErrNotFound", err)
	}
}
