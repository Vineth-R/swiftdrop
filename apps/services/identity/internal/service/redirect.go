package service

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// maxRedirectLength matches the oauth_states.redirect_to column.
const maxRedirectLength = 255

// ValidRedirectPath reports whether path is safe to redirect a browser to after
// Google sign-in: a relative path on this site such as /dashboard (API v1.1
// section 2.5). An absolute URL, a protocol-relative URL (//host), a backslash
// (some browsers treat /\host as //host) and control characters are refused,
// so the login cannot be used as an open redirect.
func ValidRedirectPath(path string) bool {
	if path == "" || len(path) > maxRedirectLength || !utf8.ValidString(path) {
		return false
	}
	if !strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") {
		return false
	}
	if strings.Contains(path, `\`) {
		return false
	}
	for _, r := range path {
		if unicode.IsControl(r) || unicode.IsSpace(r) {
			return false
		}
	}
	return true
}
