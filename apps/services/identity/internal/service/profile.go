package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Vineth-R/swiftdrop/apps/services/identity/internal/password"
	"github.com/Vineth-R/swiftdrop/apps/services/identity/internal/repository"
	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
	"github.com/Vineth-R/swiftdrop/packages/go-common/validation"
)

// Profile is the signed-in user's own view of their account (API v1.1
// section 2.7). Phone is empty when the account has none.
type Profile struct {
	ID             string
	Email          string
	FullName       string
	Phone          string
	Role           auth.Role
	AuthProvider   string
	SessionVersion int
	CreatedAt      time.Time
}

// Profile returns the account of userID, which the caller takes from a verified
// access token. It reads the database, so a user who was deleted or deactivated
// after the token was issued is refused at once instead of when it expires.
//
// Errors: ErrUserNotFound when the account no longer exists, ErrAccountDisabled
// when it was deactivated.
func (s *Service) Profile(ctx context.Context, userID string) (Profile, error) {
	user, err := s.activeUser(ctx, userID, "profile")
	if err != nil {
		return Profile{}, err
	}
	return profileOf(user), nil
}

// UpdateProfileInput is a partial update of the user's own name and phone
// (API v1.1 section 2.8). A nil field is left unchanged.
type UpdateProfileInput struct {
	FullName *string
	Phone    *string
}

// UpdateProfile changes the signed-in user's full name and phone and returns the
// account as stored. The email, the role and every other field are not
// editable here.
//
// Errors: a *ValidationError for a bad or empty request, ErrUserNotFound and
// ErrAccountDisabled as for Profile.
func (s *Service) UpdateProfile(ctx context.Context, userID string, input UpdateProfileInput) (Profile, error) {
	issues := validation.New()
	change := repository.ProfileChange{}

	if input.FullName == nil && input.Phone == nil {
		issues.Add("body", "must contain at least one of fullName, phone")
	}
	if input.FullName != nil {
		name := strings.TrimSpace(*input.FullName)
		issues.Length("fullName", name, minFullNameLength, maxFullNameLength, "must be 2 to 150 characters")
		change.FullName = &name
	}
	if input.Phone != nil {
		issues.Custom("phone", e164.MatchString(*input.Phone), "must be in E.164 format, for example +94771234567")
		change.Phone = input.Phone
	}
	if !issues.Empty() {
		return Profile{}, &ValidationError{Issues: issues.Issues()}
	}

	if _, err := s.activeUser(ctx, userID, "update profile"); err != nil {
		return Profile{}, err
	}
	updated, err := s.store.UpdateProfile(ctx, userID, change)
	if errors.Is(err, repository.ErrNotFound) {
		return Profile{}, ErrUserNotFound
	}
	if err != nil {
		return Profile{}, fmt.Errorf("update profile: %w", err)
	}
	return profileOf(updated), nil
}

// ChangePasswordInput carries the current and the new password (API v1.1
// section 2.9).
type ChangePasswordInput struct {
	CurrentPassword string
	NewPassword     string
}

// ChangePassword sets a new password for the signed-in user after checking the
// current one. On success every refresh token of the user is revoked, so every
// session ends once its short-lived access token runs out. The caller signs the
// current browser out as well.
//
// It checks the request first, then the account, then the current password:
//   - a *ValidationError names every missing field and any rule the new
//     password breaks (the same rules as registration);
//   - ErrUserNotFound and ErrAccountDisabled as for Profile;
//   - ErrPasswordNotSet for an account that signs in with Google only;
//   - ErrInvalidCredentials when the current password is wrong.
func (s *Service) ChangePassword(ctx context.Context, userID string, input ChangePasswordInput) error {
	issues := validation.New()
	issues.RequiredExact("currentPassword", input.CurrentPassword)
	if input.NewPassword == "" {
		issues.RequiredExact("newPassword", input.NewPassword)
	} else if err := password.Validate(input.NewPassword); err != nil {
		issue := "is not acceptable"
		var policy *password.PolicyError
		if errors.As(err, &policy) {
			issue = policy.Issue
		}
		issues.Add("newPassword", issue)
	}
	if !issues.Empty() {
		return &ValidationError{Issues: issues.Issues()}
	}

	user, err := s.activeUser(ctx, userID, "change password")
	if err != nil {
		return err
	}
	if user.PasswordHash == nil {
		return ErrPasswordNotSet
	}
	if !s.hasher.Matches(*user.PasswordHash, input.CurrentPassword) {
		return ErrInvalidCredentials
	}

	hash, err := s.hasher.Hash(input.NewPassword)
	if err != nil {
		return fmt.Errorf("change password: %w", err)
	}
	err = s.store.ChangePassword(ctx, userID, *user.PasswordHash, hash, s.nowUTC())
	switch {
	case errors.Is(err, repository.ErrPasswordMismatch):
		// Another request changed the password after it was checked above.
		return ErrInvalidCredentials
	case errors.Is(err, repository.ErrNotFound):
		return ErrUserNotFound
	case err != nil:
		return fmt.Errorf("change password: %w", err)
	}
	s.logger.InfoContext(ctx, "password changed", "user_id", userID)
	s.notifier.PasswordChanged(ctx, recipientOf(user), false, s.nowUTC())
	return nil
}

// activeUser loads the user and refuses one that is gone or deactivated with
// ErrUserNotFound or ErrAccountDisabled. Any other failure is wrapped with op,
// the name of the use case, for the log.
func (s *Service) activeUser(ctx context.Context, userID, op string) (repository.User, error) {
	user, err := s.store.UserByID(ctx, userID)
	if errors.Is(err, repository.ErrNotFound) {
		return repository.User{}, ErrUserNotFound
	}
	if err != nil {
		return repository.User{}, fmt.Errorf("%s: %w", op, err)
	}
	if !user.IsActive {
		return repository.User{}, ErrAccountDisabled
	}
	return user, nil
}

func profileOf(user repository.User) Profile {
	profile := Profile{
		ID:             user.ID,
		Email:          user.Email,
		FullName:       user.FullName,
		Role:           user.Role,
		AuthProvider:   user.AuthProvider,
		SessionVersion: user.SessionVersion,
		CreatedAt:      user.CreatedAt.UTC(),
	}
	if user.Phone != nil {
		profile.Phone = *user.Phone
	}
	return profile
}
