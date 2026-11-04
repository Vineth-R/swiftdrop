// Package notify sends the identity service's account emails (welcome,
// password changed, account locked and so on) with the branded templates in
// packages/go-common/emails. It implements service.Notifier.
//
// Delivery happens in the background, because an email that is slow or fails
// must never slow down or fail the login, registration or password change that
// triggered it. A failure is logged with the user ID and the kind of email,
// never with the address, the link or the message.
package notify

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/Vineth-R/swiftdrop/apps/services/identity/internal/service"
	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
	"github.com/Vineth-R/swiftdrop/packages/go-common/emails"
	"github.com/Vineth-R/swiftdrop/packages/go-common/mailer"
)

// sendTimeout bounds one background delivery.
const sendTimeout = 30 * time.Second

// Sender implements service.Notifier over a mailer.Mailer.
type Sender struct {
	mailer mailer.Mailer
	site   emails.Site
	logger *slog.Logger

	// dispatch runs a delivery. The default runs it in the background.
	dispatch func(func())
	pending  sync.WaitGroup
}

var _ service.Notifier = (*Sender)(nil)

// Option customises a Sender.
type Option func(*Sender)

// Synchronous delivers each email before the method returns. It exists so tests
// can inspect the result without waiting.
func Synchronous() Option {
	return func(s *Sender) { s.dispatch = func(job func()) { job() } }
}

// New returns a Sender. site carries the web app's address and the support
// address; every link in an email is built from it, never from a request.
func New(sender mailer.Mailer, site emails.Site, logger *slog.Logger, opts ...Option) *Sender {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	s := &Sender{mailer: sender, site: site, logger: logger}
	s.dispatch = func(job func()) {
		s.pending.Add(1)
		go func() {
			defer s.pending.Done()
			job()
		}()
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Wait blocks until background emails are delivered or have failed. Call it on
// shutdown so an email is not lost.
func (s *Sender) Wait() { s.pending.Wait() }

// Welcome implements service.Notifier.
func (s *Sender) Welcome(ctx context.Context, to service.Recipient) {
	s.send(ctx, "welcome", to, emails.Welcome(emails.WelcomeData{
		Name:   to.FullName,
		AppURL: s.site.URL("/dashboard"),
	}))
}

// PasswordChanged implements service.Notifier.
func (s *Sender) PasswordChanged(ctx context.Context, to service.Recipient, byReset bool, at time.Time) {
	s.send(ctx, "password_changed", to, emails.PasswordChanged(emails.PasswordChangedData{
		Name:     to.FullName,
		When:     emails.FormatTime(at),
		ByReset:  byReset,
		ResetURL: s.site.URL("/forgot-password"),
	}))
}

// StaffAccountCreated implements service.Notifier.
func (s *Sender) StaffAccountCreated(ctx context.Context, to service.Recipient, role auth.Role) {
	s.send(ctx, "staff_account_created", to, emails.StaffAccountCreated(emails.StaffAccountData{
		Name:     to.FullName,
		Role:     roleLabel(role),
		Email:    to.Email,
		LoginURL: s.site.URL("/login"),
	}))
}

// AccountLocked implements service.Notifier.
func (s *Sender) AccountLocked(ctx context.Context, to service.Recipient, lockedFor time.Duration) {
	s.send(ctx, "account_locked", to, emails.AccountLocked(emails.AccountLockedData{
		Name:      to.FullName,
		LockedFor: emails.FormatDuration(lockedFor),
		ResetURL:  s.site.URL("/forgot-password"),
	}))
}

// AccountStatusChanged implements service.Notifier.
func (s *Sender) AccountStatusChanged(ctx context.Context, to service.Recipient, isActive bool) {
	data := emails.AccountStatusData{Name: to.FullName, LoginURL: s.site.URL("/login")}
	if isActive {
		s.send(ctx, "account_reactivated", to, emails.AccountReactivated(data))
		return
	}
	s.send(ctx, "account_deactivated", to, emails.AccountDeactivated(data))
}

// send renders the email and delivers it in the background.
func (s *Sender) send(ctx context.Context, kind string, to service.Recipient, email emails.Email) {
	message, err := email.Message(to.Email, s.site)
	if err != nil {
		s.logger.ErrorContext(ctx, "account email could not be built", "email_kind", kind, "user_id", to.UserID, "error", err.Error())
		return
	}
	s.dispatch(func() {
		// The request is already answered, so it must not cancel the delivery.
		sendCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), sendTimeout)
		defer cancel()
		if err := s.mailer.Send(sendCtx, message); err != nil {
			s.logger.ErrorContext(sendCtx, "account email was not sent", "email_kind", kind, "user_id", to.UserID, "error", err.Error())
		}
	})
}

// roleLabel writes a role the way people say it.
func roleLabel(role auth.Role) string {
	switch role {
	case auth.RoleCourier:
		return "Courier"
	case auth.RoleDispatcher:
		return "Dispatcher"
	case auth.RoleAdmin:
		return "Administrator"
	case auth.RoleMerchant:
		return "Merchant"
	default:
		return "Customer"
	}
}
