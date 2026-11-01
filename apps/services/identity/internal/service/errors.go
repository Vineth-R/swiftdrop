package service

import "errors"

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
)

// Issue describes one invalid field. Issue text is safe to show to the client
// and never contains the submitted value.
type Issue struct {
	Field string
	Issue string
}

// ValidationError lists every invalid field of a request (422 VALIDATION_ERROR).
type ValidationError struct {
	Issues []Issue
}

func (e *ValidationError) Error() string {
	return "validation failed"
}
