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

	"github.com/Vineth-R/swiftdrop/apps/services/identity/internal/repository"
	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
	"github.com/Vineth-R/swiftdrop/packages/go-common/validation"
)

const maxVerificationTokenLength = 128

// VerifyEmail consumes an email verification token and marks the customer
// verified. Invalid, expired and used tokens share one error.
func (s *Service) VerifyEmail(ctx context.Context, token string) error {
	token = strings.TrimSpace(token)
	if token == "" || len(token) > maxVerificationTokenLength {
		return ErrVerificationTokenInvalid
	}
	userID, err := s.store.ConsumeEmailVerificationToken(ctx, hashVerificationToken(token), s.nowUTC())
	if errors.Is(err, repository.ErrVerificationTokenInvalid) {
		return ErrVerificationTokenInvalid
	}
	if err != nil {
		return fmt.Errorf("verify email: %w", err)
	}
	s.logger.InfoContext(ctx, "email verified", "user_id", userID)
	// The account is now usable. The welcome email is best effort.
	if user, err := s.store.UserByID(ctx, userID); err == nil {
		s.notifier.Welcome(ctx, recipientOf(user))
	}
	return nil
}

// ResendVerification issues a fresh verification token for an eligible
// unverified customer. It never reveals whether the account exists.
func (s *Service) ResendVerification(ctx context.Context, email string, ip netip.Addr) error {
	email = NormalizeEmail(email)
	if !validation.Email(email) {
		return &ValidationError{Issues: []Issue{{Field: "email", Issue: "must be a valid email address"}}}
	}
	user, err := s.store.UserByEmail(ctx, email)
	if errors.Is(err, repository.ErrNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("resend verification: %w", err)
	}
	if !user.IsActive || user.Role != auth.RoleCustomer || user.EmailVerified || user.PasswordHash == nil {
		return nil
	}
	token, err := newVerificationToken()
	if err != nil {
		return fmt.Errorf("resend verification: %w", err)
	}
	now := s.nowUTC()
	var requestedIP *netip.Addr
	if ip.IsValid() {
		requestedIP = &ip
	}
	if err := s.store.IssueEmailVerificationToken(ctx, repository.NewEmailVerificationToken{
		UserID:      user.ID,
		TokenHash:   hashVerificationToken(token),
		ExpiresAt:   now.Add(s.config.EmailVerificationTTL),
		RequestedIP: requestedIP,
	}, now); err != nil {
		return fmt.Errorf("resend verification: %w", err)
	}
	s.notifier.VerifyEmail(ctx, recipientOf(user), s.verificationLink(token), s.config.EmailVerificationTTL)
	return nil
}

// merchantStanding reads the merchant status of an owner and the
// administrator's reason. It fails closed with ErrServiceUnavailable.
func (s *Service) merchantStanding(ctx context.Context, userID string) (status, reason string, err error) {
	if s.merchantApproval == nil {
		return "", "", ErrServiceUnavailable
	}
	status, reason, err = s.merchantApproval.Approval(ctx, userID)
	if err != nil {
		return "", "", ErrServiceUnavailable
	}
	return status, reason, nil
}

func (s *Service) merchantApproved(ctx context.Context, userID string) (bool, error) {
	status, _, err := s.merchantStanding(ctx, userID)
	if err != nil {
		return false, err
	}
	return status == "APPROVED", nil
}

// inactiveMerchantError is the refusal for a merchant owner who is not
// approved. A PENDING, REJECTED or SUSPENDED application carries its details so
// the owner can see where it stands; any other state is the plain refusal.
func (s *Service) inactiveMerchantError(status, reason string) error {
	switch status {
	case "PENDING", "REJECTED", "SUSPENDED":
		return &MerchantInactiveError{Status: status, Reason: reason, SupportEmail: s.config.SupportEmail}
	}
	return ErrMerchantAccountInactive
}

// inactiveMerchantLogin refuses an owner whose Identity account is already
// inactive. The lookup only adds detail, so when the Merchant service cannot be
// reached the owner still gets the plain refusal rather than a 503.
func (s *Service) inactiveMerchantLogin(ctx context.Context, userID string) error {
	status, reason, err := s.merchantStanding(ctx, userID)
	if err != nil {
		return ErrMerchantAccountInactive
	}
	return s.inactiveMerchantError(status, reason)
}

func (s *Service) verificationLink(token string) string {
	return strings.TrimRight(s.config.FrontendBaseURL, "/") + "/verify-email?token=" + url.QueryEscape(token)
}

func newVerificationToken() (string, error) {
	return randomURLSafe()
}

func hashVerificationToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
