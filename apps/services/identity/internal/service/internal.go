package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Vineth-R/swiftdrop/apps/services/identity/internal/repository"
	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
	"github.com/Vineth-R/swiftdrop/packages/go-common/validation"
)

// ErrUserNotFound means no user has the requested ID (404 NOT_FOUND). Callers
// get the same error for a malformed ID, so probing reveals nothing.
var ErrUserNotFound = errors.New("user not found")

// ValidUUID reports whether value has the textual form of a UUID. Checking it
// first keeps a malformed ID from reaching PostgreSQL, which would fail the
// query instead of finding nothing.
func ValidUUID(value string) bool { return validation.UUID(value) }

// InternalStore is the persistence the internal APIs need.
// *repository.Repository satisfies it.
type InternalStore interface {
	CourierEligibility(ctx context.Context, userID string) (repository.CourierEligibility, error)
	UserContact(ctx context.Context, userID string) (repository.UserContact, error)
	CreateInactiveMerchantOwner(ctx context.Context, email, passwordHash, fullName, phone string) (repository.User, bool, error)
	ApplyMerchantLifecycleTransition(ctx context.Context, userID, transitionID string, sequence int64, operation string, now time.Time) (repository.MerchantLifecycleTransition, error)
	SessionState(ctx context.Context, userID string) (repository.SessionState, error)
	DeletePendingMerchantOwner(ctx context.Context, userID string) error
}

// InternalOption customises the internal API.
type InternalOption func(*InternalAPI)

// WithInternalHasher supplies the password hasher for merchant owner creation.
func WithInternalHasher(hasher Hasher) InternalOption {
	return func(a *InternalAPI) { a.hasher = hasher }
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
	store  InternalStore
	hasher Hasher
}

// NewInternalAPI returns an InternalAPI.
func NewInternalAPI(store InternalStore, opts ...InternalOption) (*InternalAPI, error) {
	if store == nil {
		return nil, errors.New("internal API requires a store")
	}
	api := &InternalAPI{store: store}
	for _, opt := range opts {
		opt(api)
	}
	return api, nil
}

// MerchantOwnerInput creates an inactive merchant owner.
type MerchantOwnerInput struct {
	Email    string
	Password string
	FullName string
	Phone    string
}

// MerchantOwnerResult is the created or resumed account.
type MerchantOwnerResult struct {
	UserID  string
	Created bool
}

// CreateMerchantOwner validates and creates (or resumes) an inactive MERCHANT
// account. Password material is hashed here and never stored elsewhere.
func (a *InternalAPI) CreateMerchantOwner(ctx context.Context, input MerchantOwnerInput) (MerchantOwnerResult, error) {
	if a.hasher == nil {
		return MerchantOwnerResult{}, errors.New("merchant owner creation is not configured")
	}
	email := NormalizeEmail(input.Email)
	if err := validateRegistration(RegisterInput{Email: input.Email, Password: input.Password, FullName: input.FullName, Phone: input.Phone}, email); err != nil {
		return MerchantOwnerResult{}, err
	}
	hash, err := a.hasher.Hash(input.Password)
	if err != nil {
		return MerchantOwnerResult{}, fmt.Errorf("create merchant owner: %w", err)
	}
	user, created, err := a.store.CreateInactiveMerchantOwner(ctx, email, hash, strings.TrimSpace(input.FullName), input.Phone)
	if err != nil {
		return MerchantOwnerResult{}, err
	}
	if !created {
		if user.PasswordHash == nil || !a.hasher.Matches(*user.PasswordHash, input.Password) {
			return MerchantOwnerResult{}, repository.ErrEmailTaken
		}
	}
	return MerchantOwnerResult{UserID: user.ID, Created: created}, nil
}

// LifecycleInput applies one merchant lifecycle transition.
type LifecycleInput struct {
	UserID       string
	TransitionID string
	Sequence     int64
	Operation    string
}

// LifecycleResult is the applied transition state.
type LifecycleResult struct {
	UserID          string
	TransitionID    string
	Sequence        int64
	Operation       string
	AlreadyApplied  bool
	SessionVersion  int
	IsActive        bool
	RevokedSessions int64
}

// ApplyMerchantLifecycle applies a transition exactly once.
func (a *InternalAPI) ApplyMerchantLifecycle(ctx context.Context, input LifecycleInput) (LifecycleResult, error) {
	if !ValidUUID(input.UserID) || !ValidUUID(input.TransitionID) {
		return LifecycleResult{}, ErrUserNotFound
	}
	if input.Sequence < 1 {
		return LifecycleResult{}, &ValidationError{Issues: []Issue{{Field: "sequence", Issue: "must be greater than zero"}}}
	}
	switch input.Operation {
	case "APPROVE", "REJECT", "SUSPEND":
	default:
		return LifecycleResult{}, &ValidationError{Issues: []Issue{{Field: "operation", Issue: "must be APPROVE, REJECT or SUSPEND"}}}
	}
	applied, err := a.store.ApplyMerchantLifecycleTransition(ctx, input.UserID, input.TransitionID, input.Sequence, input.Operation, time.Now().UTC())
	if errors.Is(err, repository.ErrNotFound) {
		return LifecycleResult{}, ErrUserNotFound
	}
	if err != nil {
		return LifecycleResult{}, err
	}
	return LifecycleResult{
		UserID: applied.UserID, TransitionID: applied.TransitionID, Sequence: applied.Sequence,
		Operation: applied.Operation, AlreadyApplied: applied.AlreadyApplied,
		SessionVersion: applied.SessionVersion, IsActive: applied.IsActive,
		RevokedSessions: applied.RevokedSessions,
	}, nil
}

// SessionState is the current session authority state of a user.
type SessionState struct {
	Role           auth.Role
	IsActive       bool
	SessionVersion int
}

// SessionState returns the current session authority state.
func (a *InternalAPI) SessionState(ctx context.Context, userID string) (SessionState, error) {
	if !ValidUUID(userID) {
		return SessionState{}, ErrUserNotFound
	}
	state, err := a.store.SessionState(ctx, userID)
	if errors.Is(err, repository.ErrNotFound) {
		return SessionState{}, ErrUserNotFound
	}
	if err != nil {
		return SessionState{}, fmt.Errorf("session state: %w", err)
	}
	return SessionState{Role: state.Role, IsActive: state.IsActive, SessionVersion: state.SessionVersion}, nil
}

// DeleteMerchantOwner compensates a failed merchant registration.
func (a *InternalAPI) DeleteMerchantOwner(ctx context.Context, userID string) error {
	if !ValidUUID(userID) {
		return ErrUserNotFound
	}
	err := a.store.DeletePendingMerchantOwner(ctx, userID)
	if errors.Is(err, repository.ErrNotFound) {
		return ErrUserNotFound
	}
	return err
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
