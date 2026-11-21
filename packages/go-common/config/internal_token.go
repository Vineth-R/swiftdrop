package config

import (
	"fmt"
	"strings"
)

const (
	// MinInternalTokenLength is the development minimum. Keeping it at 16
	// preserves the documented local workflow.
	MinInternalTokenLength = 16

	// MinInternalTokenLengthNonDevelopment is the minimum outside development.
	// The value must be random token material, not a placeholder.
	MinInternalTokenLengthNonDevelopment = 32

	internalTokenPlaceholderPrefix = "change-me"
)

// LoadInternalToken reads INTERNAL_TOKEN and validates it for the current
// environment. It returns the token and, when the token cannot be used, a
// problem description. The problem never contains the token value.
//
// Development keeps the existing lenient behaviour so local workflows continue
// to work: an empty or short token closes the internal endpoints. Outside
// development, an empty token, a known placeholder, or a token shorter than 32
// bytes is rejected; callers turn that problem into a startup error.
func LoadInternalToken(loader Loader, appEnv string) (token, problem string) {
	token = strings.TrimSpace(loader.String("INTERNAL_TOKEN", ""))
	if token == "" {
		return "", "INTERNAL_TOKEN is not set"
	}

	if strings.EqualFold(strings.TrimSpace(appEnv), "development") {
		if len(token) < MinInternalTokenLength {
			return "", fmt.Sprintf("INTERNAL_TOKEN is shorter than %d characters", MinInternalTokenLength)
		}
		return token, ""
	}

	lower := strings.ToLower(token)
	if strings.HasPrefix(lower, internalTokenPlaceholderPrefix) || strings.HasPrefix(lower, "changeme") {
		return "", "INTERNAL_TOKEN must not be the example placeholder outside development"
	}
	if len(token) < MinInternalTokenLengthNonDevelopment {
		return "", fmt.Sprintf("INTERNAL_TOKEN must be at least %d bytes outside development", MinInternalTokenLengthNonDevelopment)
	}
	return token, ""
}
