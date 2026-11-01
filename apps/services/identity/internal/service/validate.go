package service

import (
	"errors"
	"net/mail"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/Vineth-R/swiftdrop/apps/services/identity/internal/password"
)

const (
	maxEmailLength    = 255
	minFullNameLength = 2
	maxFullNameLength = 150
)

// e164 matches the E.164 phone format: a plus sign, a non-zero first digit and
// up to 15 digits in total.
var e164 = regexp.MustCompile(`^\+[1-9][0-9]{1,14}$`)

// normalizeEmail trims and lower-cases an email. Accounts are matched without
// regard to letter case (DB v1.1 section 3.2).
func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// validEmail reports whether email is a plain address such as a@b.lk, without a
// display name or angle brackets.
func validEmail(email string) bool {
	if email == "" || len(email) > maxEmailLength {
		return false
	}
	address, err := mail.ParseAddress(email)
	if err != nil || address.Address != email {
		return false
	}
	_, domain, found := strings.Cut(email, "@")
	return found && strings.Contains(domain, ".")
}

func validateRegistration(input RegisterInput, email string) error {
	var issues []Issue

	if !validEmail(email) {
		issues = append(issues, Issue{Field: "email", Issue: "must be a valid email address"})
	}

	if err := password.Validate(input.Password); err != nil {
		var policy *password.PolicyError
		if errors.As(err, &policy) {
			issues = append(issues, Issue{Field: "password", Issue: policy.Issue})
		} else {
			issues = append(issues, Issue{Field: "password", Issue: "is not acceptable"})
		}
	}

	if length := utf8.RuneCountInString(strings.TrimSpace(input.FullName)); length < minFullNameLength || length > maxFullNameLength {
		issues = append(issues, Issue{Field: "fullName", Issue: "must be 2 to 150 characters"})
	}

	if !e164.MatchString(input.Phone) {
		issues = append(issues, Issue{Field: "phone", Issue: "must be in E.164 format, for example +94771234567"})
	}

	if len(issues) > 0 {
		return &ValidationError{Issues: issues}
	}
	return nil
}
