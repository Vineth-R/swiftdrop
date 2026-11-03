package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/netip"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/Vineth-R/swiftdrop/apps/services/identity/internal/password"
	"github.com/Vineth-R/swiftdrop/apps/services/identity/internal/repository"
	"github.com/Vineth-R/swiftdrop/packages/go-common/mailer"
)

const (
	// maxResetTokenLength bounds the work done on untrusted input. A real token
	// is 43 characters.
	maxResetTokenLength = 128

	// emailSendTimeout bounds one background email delivery.
	emailSendTimeout = 30 * time.Second

	resetEmailSubject = "Reset your SwiftDrop password"
)

// ErrResetTokenInvalid means the reset token is unknown, expired or already
// used (400 INVALID_OR_EXPIRED_RESET_TOKEN).
var ErrResetTokenInvalid = errors.New("reset token invalid")

// ResetStore is the persistence password reset needs. *repository.Repository
// satisfies it.
type ResetStore interface {
	UserByEmail(ctx context.Context, email string) (repository.User, error)
	IssuePasswordResetToken(ctx context.Context, token repository.NewPasswordResetToken, now time.Time) error
	PasswordResetTokenUsable(ctx context.Context, tokenHash string, now time.Time) (bool, error)
	ResetPasswordWithToken(ctx context.Context, tokenHash, newPasswordHash string, now time.Time) (string, error)
}

// PasswordReset implements forgot and reset password (API v1.1 sections 2.16
// and 2.17). It reuses Service for the clock, hasher and logger.
type PasswordReset struct {
	base          *Service
	store         ResetStore
	mailer        mailer.Mailer
	frontend      string // trusted frontend base URL, no trailing slash
	resetTokenTTL time.Duration

	// dispatch runs email delivery. The default runs it in the background so the
	// response time does not reveal whether an account exists.
	dispatch func(func())
	pending  sync.WaitGroup
}

// PasswordResetOption customises a PasswordReset.
type PasswordResetOption func(*PasswordReset)

// WithSynchronousEmail delivers email before returning. It exists so tests can
// inspect the result without waiting.
func WithSynchronousEmail() PasswordResetOption {
	return func(p *PasswordReset) { p.dispatch = func(job func()) { job() } }
}

// NewPasswordReset returns a PasswordReset. frontendBaseURL is the trusted
// public address of the web app, from configuration; reset links are built from
// it and never from a request header.
func NewPasswordReset(
	base *Service,
	store ResetStore,
	sender mailer.Mailer,
	frontendBaseURL string,
	resetTokenTTL time.Duration,
	opts ...PasswordResetOption,
) (*PasswordReset, error) {
	if base == nil || store == nil || sender == nil {
		return nil, errors.New("password reset requires a service, a store and a mailer")
	}
	if resetTokenTTL <= 0 {
		return nil, errors.New("password reset requires a positive token lifetime")
	}
	parsed, err := url.Parse(frontendBaseURL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, errors.New("frontend base URL must be an absolute http or https URL")
	}

	p := &PasswordReset{
		base:          base,
		store:         store,
		mailer:        sender,
		frontend:      strings.TrimRight(frontendBaseURL, "/"),
		resetTokenTTL: resetTokenTTL,
	}
	p.dispatch = func(job func()) {
		p.pending.Add(1)
		go func() {
			defer p.pending.Done()
			job()
		}()
	}
	for _, opt := range opts {
		opt(p)
	}
	return p, nil
}

// Wait blocks until background emails have been delivered or have failed. Call
// it during shutdown so a reset email is not lost.
func (p *PasswordReset) Wait() { p.pending.Wait() }

// ForgotPasswordInput is a request for a reset link. IP is stored with the
// token for abuse review.
type ForgotPasswordInput struct {
	Email string
	IP    netip.Addr
}

// ForgotPassword starts a password reset. It returns a *ValidationError for a
// malformed email. For every well-formed email it returns nil, whether or not
// the account exists, so registered addresses are not revealed. Accounts that
// cannot use a password (Google-only) and deactivated accounts silently get no
// token and no email.
func (p *PasswordReset) ForgotPassword(ctx context.Context, input ForgotPasswordInput) error {
	email := NormalizeEmail(input.Email)
	if !ValidEmail(email) {
		return &ValidationError{Issues: []Issue{{Field: "email", Issue: "must be a valid email address"}}}
	}

	user, err := p.store.UserByEmail(ctx, email)
	if errors.Is(err, repository.ErrNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("forgot password: %w", err)
	}
	if !user.IsActive || user.PasswordHash == nil {
		return nil
	}

	token, err := newResetToken()
	if err != nil {
		return fmt.Errorf("forgot password: %w", err)
	}
	now := p.base.nowUTC()
	var ip *netip.Addr
	if input.IP.IsValid() {
		ip = &input.IP
	}
	if err := p.store.IssuePasswordResetToken(ctx, repository.NewPasswordResetToken{
		UserID:      user.ID,
		TokenHash:   hashResetToken(token),
		ExpiresAt:   now.Add(p.resetTokenTTL),
		RequestedIP: ip,
	}, now); err != nil {
		return fmt.Errorf("forgot password: %w", err)
	}

	message := resetEmail(user.Email, user.FullName, p.resetLink(token))
	userID := user.ID
	p.dispatch(func() {
		// The request is already answered, so it must not cancel the delivery.
		sendCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), emailSendTimeout)
		defer cancel()
		if err := p.mailer.Send(sendCtx, message); err != nil {
			// The log carries the user ID only: no email address, link or token.
			p.base.logger.ErrorContext(sendCtx, "password reset email was not sent", "user_id", userID, "error", err.Error())
		}
	})
	return nil
}

// ResetPasswordInput carries the emailed token and the new password.
type ResetPasswordInput struct {
	Token       string
	NewPassword string
}

// ResetPassword sets a new password using a reset token, checking in the order
// of API v1.1 section 2.17: first the token, then the password.
//
// A missing, unknown, expired or used token returns ErrResetTokenInvalid, with
// no way for the caller to tell which. A password that breaks the registration
// rules returns a *ValidationError and leaves the token unspent, so the user can
// try again with the same link. On success the token is spent and every session
// of the user ends.
func (p *PasswordReset) ResetPassword(ctx context.Context, input ResetPasswordInput) error {
	if strings.TrimSpace(input.Token) == "" || len(input.Token) > maxResetTokenLength {
		return ErrResetTokenInvalid
	}
	tokenHash := hashResetToken(input.Token)

	usable, err := p.store.PasswordResetTokenUsable(ctx, tokenHash, p.base.nowUTC())
	if err != nil {
		return fmt.Errorf("reset password: %w", err)
	}
	if !usable {
		return ErrResetTokenInvalid
	}

	if err := password.Validate(input.NewPassword); err != nil {
		issue := "is not acceptable"
		var policy *password.PolicyError
		if errors.As(err, &policy) {
			issue = policy.Issue
		}
		return &ValidationError{Issues: []Issue{{Field: "newPassword", Issue: issue}}}
	}

	hash, err := p.base.hasher.Hash(input.NewPassword)
	if err != nil {
		return fmt.Errorf("reset password: %w", err)
	}
	// The check above was only a read. Spending the token repeats it under a lock,
	// so two requests with the same token still cannot both succeed.
	userID, err := p.store.ResetPasswordWithToken(ctx, tokenHash, hash, p.base.nowUTC())
	if errors.Is(err, repository.ErrResetTokenInvalid) {
		return ErrResetTokenInvalid
	}
	if err != nil {
		return fmt.Errorf("reset password: %w", err)
	}
	p.base.logger.InfoContext(ctx, "password reset completed", "user_id", userID)
	return nil
}

func (p *PasswordReset) resetLink(token string) string {
	return p.frontend + "/reset-password?token=" + url.QueryEscape(token)
}

func resetEmail(to, fullName, link string) mailer.Message {
	text := "Hello " + fullName + ",\r\n\r\n" +
		"We received a request to reset your SwiftDrop password. Open this link to choose a new one:\r\n\r\n" +
		link + "\r\n\r\n" +
		"The link works once and expires in 30 minutes. If you did not ask for this, you can ignore this email; your password has not changed.\r\n"
	html := "<p>Hello " + htmlEscape(fullName) + ",</p>" +
		"<p>We received a request to reset your SwiftDrop password. " +
		`<a href="` + htmlEscape(link) + `">Choose a new password</a>.</p>` +
		"<p>The link works once and expires in 30 minutes. If you did not ask for this, you can ignore this email; your password has not changed.</p>"
	return mailer.Message{To: to, Subject: resetEmailSubject, Text: text, HTML: html}
}

func htmlEscape(value string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&#34;", "'", "&#39;").Replace(value)
}

// newResetToken returns a random reset token: 32 bytes from the system random
// generator, 43 URL-safe characters.
func newResetToken() (string, error) {
	return randomURLSafe()
}

// hashResetToken returns the SHA-256 hex digest that is stored in place of the
// token (DB v1.1 section 3.7).
func hashResetToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
