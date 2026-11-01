package service

import (
	"context"
	"errors"
	"fmt"
	"regexp"

	"github.com/Vineth-R/swiftdrop/apps/services/identity/internal/repository"
	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
)

// ErrUserNotFound means no user has the requested ID (404 NOT_FOUND). Callers
// get the same error for a malformed ID, so probing reveals nothing.
var ErrUserNotFound = errors.New("user not found")

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// ValidUUID reports whether value has the textual form of a UUID. Checking it
// first keeps a malformed ID from reaching PostgreSQL, which would fail the
// query instead of finding nothing.
func ValidUUID(value string) bool { return uuidPattern.MatchString(value) }

// InternalStore is the persistence the internal APIs need.
// *repository.Repository satisfies it.
type InternalStore interface {
	CourierEligibility(ctx context.Context, userID string) (repository.CourierEligibility, error)
	UserContact(ctx context.Context, userID string) (repository.UserContact, error)
}

// CourierEligibility answers whether a user may be assigned parcels.
type CourierEligibility struct {
	IsActive    bool
	IsAvailable bool
	Role        auth.Role
}

// UserContact is the contact detail set returned to other services. Phone is
// empty when the user has none.
type UserContact struct {
	FullName string
	Email    string
	Phone    string
}

// InternalAPI implements the service-to-service lookups of API v1.1 section 13.
// It decides nothing about eligibility itself: the caller combines isActive,
// isAvailable and role.
type InternalAPI struct {
	store InternalStore
}

// NewInternalAPI returns an InternalAPI.
func NewInternalAPI(store InternalStore) (*InternalAPI, error) {
	if store == nil {
		return nil, errors.New("internal API requires a store")
	}
	return &InternalAPI{store: store}, nil
}

// CourierEligibility returns the facts for the user with courierID. Any
// existing user is answered, with their real role, so the caller can refuse a
// non-COURIER. ErrUserNotFound is returned for an unknown or malformed ID.
func (a *InternalAPI) CourierEligibility(ctx context.Context, courierID string) (CourierEligibility, error) {
	if !ValidUUID(courierID) {
		return CourierEligibility{}, ErrUserNotFound
	}
	found, err := a.store.CourierEligibility(ctx, courierID)
	if errors.Is(err, repository.ErrNotFound) {
		return CourierEligibility{}, ErrUserNotFound
	}
	if err != nil {
		return CourierEligibility{}, fmt.Errorf("courier eligibility: %w", err)
	}
	return CourierEligibility{IsActive: found.IsActive, IsAvailable: found.IsAvailable, Role: found.Role}, nil
}

// UserContact returns the name, email and phone of the user. ErrUserNotFound is
// returned for an unknown or malformed ID.
func (a *InternalAPI) UserContact(ctx context.Context, userID string) (UserContact, error) {
	if !ValidUUID(userID) {
		return UserContact{}, ErrUserNotFound
	}
	found, err := a.store.UserContact(ctx, userID)
	if errors.Is(err, repository.ErrNotFound) {
		return UserContact{}, ErrUserNotFound
	}
	if err != nil {
		return UserContact{}, fmt.Errorf("user contact: %w", err)
	}
	contact := UserContact{FullName: found.FullName, Email: found.Email}
	if found.Phone != nil {
		contact.Phone = *found.Phone
	}
	return contact, nil
}
