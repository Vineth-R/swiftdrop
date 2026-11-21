package service

import (
	"errors"
	"regexp"
	"strings"

	"github.com/Vineth-R/swiftdrop/apps/services/identity/internal/password"
	"github.com/Vineth-R/swiftdrop/packages/go-common/validation"
)

const (
	minFullNameLength = 2
	maxFullNameLength = 150
)

// e164 matches the E.164 phone format: a plus sign, a non-zero first digit and
// up to 15 digits in total.
var e164 = regexp.MustCompile(`^\+[1-9][0-9]{1,14}$`)

// NormalizeEmail trims and lower-cases an email. Accounts are matched without
// regard to letter case (DB v1.1 section 3.2).
func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// ValidEmail reports whether email is a plain address such as a@b.lk, without a
// display name or angle brackets.
func ValidEmail(email string) bool {
	return validation.Email(email)
}

func validateRegistration(input RegisterInput, email string) error {
	issues := validation.New()

	issues.Email("email", email)

	if err := password.Validate(input.Password); err != nil {
		var policy *password.PolicyError
		if errors.As(err, &policy) {
			issues.Add("password", policy.Issue)
		} else {
			issues.Add("password", "is not acceptable")
		}
	}

	issues.Length("fullName", strings.TrimSpace(input.FullName), minFullNameLength, maxFullNameLength, "must be 2 to 150 characters")

	issues.Custom("phone", e164.MatchString(input.Phone), "must be in E.164 format, for example +94771234567")

	if !issues.Empty() {
		return &ValidationError{Issues: issues.Issues()}
	}
	return nil
}
