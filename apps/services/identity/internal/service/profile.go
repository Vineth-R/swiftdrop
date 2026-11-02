package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Vineth-R/swiftdrop/apps/services/identity/internal/repository"
	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
)

// Profile is the signed-in user's own view of their account (API v1.1
// section 2.7). Phone is empty when the account has none.
type Profile struct {
	ID           string
	Email        string
	FullName     string
	Phone        string
	Role         auth.Role
	AuthProvider string
	CreatedAt    time.Time
}

// Profile returns the account of userID, which the caller takes from a verified
// access token. It reads the database, so a user who was deleted or deactivated
// after the token was issued is refused at once instead of when it expires.
//
// Errors: ErrUserNotFound when the account no longer exists, ErrAccountDisabled
// when it was deactivated.
func (s *Service) Profile(ctx context.Context, userID string) (Profile, error) {
	user, err := s.store.UserByID(ctx, userID)
	if errors.Is(err, repository.ErrNotFound) {
		return Profile{}, ErrUserNotFound
	}
	if err != nil {
		return Profile{}, fmt.Errorf("profile: %w", err)
	}
	if !user.IsActive {
		return Profile{}, ErrAccountDisabled
	}

	profile := Profile{
		ID:           user.ID,
		Email:        user.Email,
		FullName:     user.FullName,
		Role:         user.Role,
		AuthProvider: user.AuthProvider,
		CreatedAt:    user.CreatedAt.UTC(),
	}
	if user.Phone != nil {
		profile.Phone = *user.Phone
	}
	return profile, nil
}
