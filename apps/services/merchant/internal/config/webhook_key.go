package config

import (
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	sharedconfig "github.com/Vineth-R/swiftdrop/packages/go-common/config"
)

const (
	// WebhookSecretKeyBytes is the AES-256 key length used by secretbox.
	WebhookSecretKeyBytes = 32

	webhookSecretKeyHexLength = WebhookSecretKeyBytes * 2
)

// LoadWebhookSecretKey reads and validates WEBHOOK_SECRET_KEY.
//
// An empty value is valid and keeps the webhook settings endpoints fail closed;
// the service starts but those routes answer 503. A configured value must be
// exactly 64 hexadecimal characters (32 bytes) and must not be a placeholder.
// The rule is the same in every environment: a key that cannot decrypt stored
// secrets is never accepted, and a missing key never silently enables the
// feature.
func LoadWebhookSecretKey(loader sharedconfig.Loader) (string, error) {
	key := strings.TrimSpace(loader.String("WEBHOOK_SECRET_KEY", ""))
	if key == "" {
		return "", nil
	}

	if len(key) != webhookSecretKeyHexLength {
		return "", fmt.Errorf("WEBHOOK_SECRET_KEY must be exactly %d hexadecimal characters (32 bytes)", webhookSecretKeyHexLength)
	}
	decoded, err := hex.DecodeString(key)
	if err != nil || len(decoded) != WebhookSecretKeyBytes {
		return "", errors.New("WEBHOOK_SECRET_KEY must be exactly 64 hexadecimal characters (32 bytes)")
	}
	if placeholderWebhookSecretKey(key) {
		return "", errors.New("WEBHOOK_SECRET_KEY must not be a placeholder")
	}
	return key, nil
}

// placeholderWebhookSecretKey rejects the documented example values. A key of
// all zeros is the common "present but unset" placeholder; the change-me forms
// are rejected for consistency with the other shared secrets.
func placeholderWebhookSecretKey(key string) bool {
	lower := strings.ToLower(key)
	if strings.HasPrefix(lower, "change-me") || strings.HasPrefix(lower, "changeme") {
		return true
	}
	return strings.Trim(lower, "0") == ""
}
