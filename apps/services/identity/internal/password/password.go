// Package password hashes and checks account passwords with bcrypt and applies
// the registration password policy (API v1.1 section 2.1).
package password

import (
	"errors"
	"fmt"
	"unicode"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
)

const (
	// DefaultCost is the bcrypt work factor used in production.
	DefaultCost = 12

	// MinLength is the shortest accepted password, in characters.
	MinLength = 8

	// MaxBytes is bcrypt's input limit. Longer passwords are rejected rather
	// than silently truncated.
	MaxBytes = 72
)

// PolicyError reports why a password does not meet the policy. Issue is safe
// to return to the client and never contains the password.
type PolicyError struct {
	Issue string
}

func (e *PolicyError) Error() string {
	return "password policy: " + e.Issue
}

// Validate checks the password policy: at least 8 characters with an upper
// case letter, a lower case letter and a digit, and at most 72 bytes.
func Validate(plain string) error {
	if utf8.RuneCountInString(plain) < MinLength {
		return &PolicyError{Issue: fmt.Sprintf("must be at least %d characters", MinLength)}
	}
	if len(plain) > MaxBytes {
		return &PolicyError{Issue: fmt.Sprintf("must be at most %d bytes", MaxBytes)}
	}

	var upper, lower, digit bool
	for _, char := range plain {
		switch {
		case unicode.IsUpper(char):
			upper = true
		case unicode.IsLower(char):
			lower = true
		case unicode.IsDigit(char):
			digit = true
		}
	}
	if !upper || !lower || !digit {
		return &PolicyError{Issue: "must contain an upper case letter, a lower case letter and a digit"}
	}
	return nil
}

// Hasher hashes and verifies passwords at a fixed bcrypt cost.
type Hasher struct {
	cost int
	// decoy is a valid hash of a throwaway value. Comparing against it costs the
	// same as a real check, so unknown accounts are not distinguishable by timing.
	decoy []byte
}

// NewHasher returns a Hasher. A cost of zero selects DefaultCost.
func NewHasher(cost int) (*Hasher, error) {
	if cost == 0 {
		cost = DefaultCost
	}
	if cost < bcrypt.MinCost || cost > bcrypt.MaxCost {
		return nil, fmt.Errorf("bcrypt cost must be between %d and %d", bcrypt.MinCost, bcrypt.MaxCost)
	}

	decoy, err := bcrypt.GenerateFromPassword([]byte("swiftdrop-decoy-password"), cost)
	if err != nil {
		return nil, fmt.Errorf("prepare decoy hash: %w", err)
	}
	return &Hasher{cost: cost, decoy: decoy}, nil
}

// Hash returns the bcrypt hash of plain.
func (h *Hasher) Hash(plain string) (string, error) {
	if len(plain) > MaxBytes {
		return "", errors.New("password is longer than the bcrypt limit")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(plain), h.cost)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}
	return string(hash), nil
}

// Matches reports whether plain matches hash. Any failure, including a
// malformed hash, is a mismatch.
func (h *Hasher) Matches(hash, plain string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}

// Burn spends the time of one password check without a real hash. Call it when
// there is no account to compare against, so the response time does not reveal
// whether the email exists.
func (h *Hasher) Burn(plain string) {
	_ = bcrypt.CompareHashAndPassword(h.decoy, []byte(plain))
}
