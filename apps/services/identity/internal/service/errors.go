package service

import (
	"errors"

	"github.com/Vineth-R/swiftdrop/packages/go-common/validation"
)

var (
	// ErrInvalidCredentials is returned for an unknown email, a wrong password
	// and a password-less (Google-only) account alike, so callers cannot tell
	// which one it was (API v1.1 section 2.2, 401 INVALID_CREDENTIALS).
	ErrInvalidCredentials = errors.New("invalid credentials")

	// ErrAccountLocked means too many failed logins (423 ACCOUNT_LOCKED).
	ErrAccountLocked = errors.New("account is locked")

	// ErrAccountDisabled means the account was deactivated (403 ACCOUNT_DISABLED).
	ErrAccountDisabled = errors.New("account is disabled")

	// ErrEmailAlreadyExists means the email is registered (409 EMAIL_ALREADY_EXISTS).
	ErrEmailAlreadyExists = errors.New("email already exists")

	// ErrRefreshTokenInvalid means the refresh token is unknown, expired,
	// revoked or already used (401 REFRESH_TOKEN_INVALID).
	ErrRefreshTokenInvalid = errors.New("refresh token invalid")

	// ErrPasswordNotSet means the account has no password to change, because it
	// signs in with Google only (API v1.1 section 2.9, 409 PASSWORD_NOT_SET).
	ErrPasswordNotSet = errors.New("account has no password")

	// ErrEmailVerificationRequired means a customer must verify their email
	// before a session may be issued (403 EMAIL_VERIFICATION_REQUIRED).
	ErrEmailVerificationRequired = errors.New("email verification is required")

	// ErrVerificationTokenInvalid covers unknown, expired, reused and malformed
	// verification tokens (400 INVALID_OR_EXPIRED_VERIFICATION_TOKEN).
	ErrVerificationTokenInvalid = errors.New("email verification token is invalid")

	// ErrMerchantAccountInactive means a merchant account is not approved or is
	// suspended (403 MERCHANT_ACCOUNT_INACTIVE).
	ErrMerchantAccountInactive = errors.New("merchant account is inactive")

	// ErrServiceUnavailable means a required internal dependency could not be
	// reached and the operation failed closed (503 SERVICE_UNAVAILABLE).
	ErrServiceUnavailable = errors.New("service unavailable")
)

// MerchantInactiveError is ErrMerchantAccountInactive with what the owner may
// be told about it. It is returned only after the correct password, so the
// status and the administrator's reason are not exposed to guessers. Reason is
// empty when the administrator gave none; SupportEmail is empty when no support
// address is configured.
type MerchantInactiveError struct {
	Status       string
	Reason       string
	SupportEmail string
}

func (e *MerchantInactiveError) Error() string { return ErrMerchantAccountInactive.Error() }

// Is makes errors.Is(err, ErrMerchantAccountInactive) true, so callers that do
// not need the details keep working.
func (e *MerchantInactiveError) Is(target error) bool { return target == ErrMerchantAccountInactive }

// Issue describes one invalid field. It is the shared validation issue so
// services and HTTP helpers use one shape.
type Issue = validation.Issue

// ValidationError lists every invalid field of a request (422 VALIDATION_ERROR).
type ValidationError struct {
	Issues []Issue
}

func (e *ValidationError) Error() string {
	return "validation failed"
}
