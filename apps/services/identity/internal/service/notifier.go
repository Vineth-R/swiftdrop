package service

import (
	"context"
	"time"

	"github.com/Vineth-R/swiftdrop/apps/services/identity/internal/repository"
	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
)

// Recipient is who an account email is for. It carries what an email needs and
// nothing else: never a password hash or a token.
type Recipient struct {
	UserID   string
	Email    string
	FullName string
}

// Notifier tells people about events on their account by email.
//
// Its methods return nothing and must not block the request that triggered
// them: an email that cannot be sent is the implementation's to log, never a
// reason to fail a registration or a password change. Implementations must be
// safe for concurrent use. The default does nothing, which is what a service
// without an SMTP server gets.
type Notifier interface {
	// Welcome follows a new customer account, made by email or with Google.
	Welcome(ctx context.Context, to Recipient)
	// PasswordChanged follows a password change or reset. byReset is true for a
	// reset link and false for a change from the profile.
	PasswordChanged(ctx context.Context, to Recipient, byReset bool, at time.Time)
	// StaffAccountCreated follows an administrator creating a staff account. It
	// never carries the password.
	StaffAccountCreated(ctx context.Context, to Recipient, role auth.Role)
	// AccountLocked follows the failed login that locked the account.
	AccountLocked(ctx context.Context, to Recipient, lockedFor time.Duration)
	// AccountStatusChanged follows an administrator activating or deactivating
	// an account.
	AccountStatusChanged(ctx context.Context, to Recipient, isActive bool)
}

// WithNotifier sets who sends account emails. Without it no email is sent.
func WithNotifier(notifier Notifier) Option {
	return func(s *Service) {
		if notifier != nil {
			s.notifier = notifier
		}
	}
}

// noNotifier is the default: no SMTP server, no account email.
type noNotifier struct{}

func (noNotifier) Welcome(context.Context, Recipient)                          {}
func (noNotifier) PasswordChanged(context.Context, Recipient, bool, time.Time) {}
func (noNotifier) StaffAccountCreated(context.Context, Recipient, auth.Role)   {}
func (noNotifier) AccountLocked(context.Context, Recipient, time.Duration)     {}
func (noNotifier) AccountStatusChanged(context.Context, Recipient, bool)       {}

func recipientOf(user repository.User) Recipient {
	return Recipient{UserID: user.ID, Email: user.Email, FullName: user.FullName}
}
