package config

import (
	sharedconfig "github.com/Vineth-R/swiftdrop/packages/go-common/config"
)

// MinInternalTokenLength is the shortest INTERNAL_TOKEN accepted. A shorter
// secret is treated as not configured, so the internal APIs stay closed.
const MinInternalTokenLength = 16

// LoadInternalToken reads INTERNAL_TOKEN, the shared secret callers send in
// X-Internal-Token (API v1.1 section 1.2). It returns the token and, when the
// internal APIs cannot be opened safely, the reason. The reason names the
// setting, never its value. The token is returned empty whenever there is a
// problem, so no caller can accidentally use a weak one.
func LoadInternalToken(loader sharedconfig.Loader) (token, problem string) {
	token = loader.String("INTERNAL_TOKEN", "")
	switch {
	case token == "":
		return "", "INTERNAL_TOKEN is not set"
	case len(token) < MinInternalTokenLength:
		return "", "INTERNAL_TOKEN is shorter than 16 characters"
	}
	return token, ""
}
