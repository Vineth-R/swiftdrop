package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
)

// CourierEligibility is what the Parcel service needs to know before assigning
// a parcel to a user (API v1.1 section 13).
type CourierEligibility struct {
	IsActive    bool
	IsAvailable bool
	Role        auth.Role
}

// UserContact is how the Notification service reaches a user (API v1.1 section
// 13). Phone is nil when the user has none, as Google sign-ups do not.
type UserContact struct {
	FullName string
	Email    string
	Phone    *string
}

// CourierEligibility returns the active flag, the courier availability and the
// role of the user. A user without a courier profile is not available. It
// returns ErrNotFound when no user has the ID; the caller decides what a
// non-COURIER role means.
func (r *Repository) CourierEligibility(ctx context.Context, userID string) (CourierEligibility, error) {
	var (
		eligibility CourierEligibility
		role        string
	)
	err := r.db.QueryRow(ctx, `
		SELECT u.is_active, COALESCE(cp.is_available, false), r.name
		FROM identity.users u
		JOIN identity.roles r ON r.id = u.role_id
		LEFT JOIN identity.courier_profiles cp ON cp.user_id = u.id
		WHERE u.id = $1`,
		userID,
	).Scan(&eligibility.IsActive, &eligibility.IsAvailable, &role)
	if errors.Is(err, pgx.ErrNoRows) {
		return CourierEligibility{}, ErrNotFound
	}
	if err != nil {
		return CourierEligibility{}, fmt.Errorf("reading courier eligibility: %w", err)
	}
	eligibility.Role = auth.Role(role)
	return eligibility, nil
}

// UserContact returns the name, email and phone of the user, or ErrNotFound.
func (r *Repository) UserContact(ctx context.Context, userID string) (UserContact, error) {
	var contact UserContact
	err := r.db.QueryRow(ctx, `
		SELECT full_name, email, phone
		FROM identity.users
		WHERE id = $1`,
		userID,
	).Scan(&contact.FullName, &contact.Email, &contact.Phone)
	if errors.Is(err, pgx.ErrNoRows) {
		return UserContact{}, ErrNotFound
	}
	if err != nil {
		return UserContact{}, fmt.Errorf("reading user contact: %w", err)
	}
	return contact, nil
}
