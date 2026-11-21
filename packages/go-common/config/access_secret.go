package config

import (
	"errors"
	"fmt"
	"strings"
)

// MinAccessSecretBytes is the minimum signing-secret length outside
// development. HS256 keys should be at least 32 random bytes.
const MinAccessSecretBytes = 32

// LoadAccessSecret reads JWT_ACCESS_SECRET and validates it for appEnv.
// Development preserves the documented local workflow and may use a placeholder
// or a short value. Outside development a configured secret must be at least 32
// bytes and must not be a known placeholder. An empty value is allowed only when
// required is false; callers that leave it empty must keep their protected
// routes fail closed.
func LoadAccessSecret(loader Loader, appEnv string, required bool) (string, error) {
	secret := strings.TrimSpace(loader.String("JWT_ACCESS_SECRET", ""))

	if strings.EqualFold(strings.TrimSpace(appEnv), "development") {
		if required && secret == "" {
			return "", errors.New("JWT_ACCESS_SECRET is required")
		}
		return secret, nil
	}

	if secret == "" {
		if required {
			return "", errors.New("JWT_ACCESS_SECRET is required outside development")
		}
		return "", nil
	}

	lower := strings.ToLower(secret)
	if strings.HasPrefix(lower, "change-me") || strings.HasPrefix(lower, "changeme") {
		return "", errors.New("JWT_ACCESS_SECRET must not be the example placeholder outside development")
	}
	if len(secret) < MinAccessSecretBytes {
		return "", fmt.Errorf("JWT_ACCESS_SECRET must be at least %d bytes outside development", MinAccessSecretBytes)
	}
	return secret, nil
}
