// Package businessreg owns the merchant business-registration-number invariant.
// It is shared by API validation and repository persistence so no creation path
// can bypass it.
package businessreg

import (
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"
)

var ErrInvalid = errors.New("business registration number is invalid")

// Normalize trims the value, collapses internal whitespace and upper-cases it.
// The canonical form is documented in the merchant API contract.
func Normalize(value string) string {
	fields := strings.Fields(strings.ToUpper(strings.TrimSpace(value)))
	return strings.Join(fields, " ")
}

// Valid reports whether value is 3-50 characters of letters, digits, spaces,
// hyphens, slashes or dots, with at least one digit.
func Valid(value string) bool {
	if length := utf8.RuneCountInString(value); length < 3 || length > 50 {
		return false
	}
	hasDigit := false
	for _, char := range value {
		if unicode.IsDigit(char) {
			hasDigit = true
		}
		if !unicode.IsLetter(char) && !unicode.IsDigit(char) && char != ' ' && char != '-' && char != '/' && char != '.' {
			return false
		}
	}
	return hasDigit
}

// NormalizeValid returns the canonical value or ErrInvalid.
func NormalizeValid(value string) (string, error) {
	normalized := Normalize(value)
	if !Valid(normalized) {
		return "", ErrInvalid
	}
	return normalized, nil
}
