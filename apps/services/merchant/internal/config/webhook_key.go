package config

import (
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	sharedconfig "github.com/Vineth-R/swiftdrop/packages/go-common/config"
	"github.com/Vineth-R/swiftdrop/packages/go-common/secretbox"
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
	if err := validateWebhookSecretKey(key); err != nil {
		return "", err
	}
	return key, nil
}

func validateWebhookSecretKey(key string) error {
	if len(key) != webhookSecretKeyHexLength {
		return fmt.Errorf("WEBHOOK_SECRET_KEY must be exactly %d hexadecimal characters (32 bytes)", webhookSecretKeyHexLength)
	}
	decoded, err := hex.DecodeString(key)
	if err != nil || len(decoded) != WebhookSecretKeyBytes {
		return errors.New("WEBHOOK_SECRET_KEY must be exactly 64 hexadecimal characters (32 bytes)")
	}
	if placeholderWebhookSecretKey(key) {
		return errors.New("WEBHOOK_SECRET_KEY must not be a placeholder")
	}
	return nil
}

// LoadWebhookKeyring reads the active key id and the optional previous keys.
// The active key keeps the existing WEBHOOK_SECRET_KEY name. Previous keys are
// given as a comma-separated list of id=hex64 entries in WEBHOOK_SECRET_KEYS
// and are used only for decryption during a rotation.
func LoadWebhookKeyring(loader sharedconfig.Loader) (string, string, map[string]string, error) {
	active, err := LoadWebhookSecretKey(loader)
	if err != nil {
		return "", "", nil, err
	}
	activeID := strings.TrimSpace(loader.String("WEBHOOK_SECRET_KEY_ID", "k1"))
	if active == "" {
		return activeID, "", nil, nil
	}
	if !secretbox.ValidKeyID(activeID) {
		return "", "", nil, errors.New("WEBHOOK_SECRET_KEY_ID must be 1-32 letters, digits, underscores or hyphens")
	}

	previous := map[string]string{}
	for _, entry := range strings.Split(loader.String("WEBHOOK_SECRET_KEYS", ""), ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		id, key, found := strings.Cut(entry, "=")
		id = strings.TrimSpace(id)
		key = strings.TrimSpace(key)
		if !found || !secretbox.ValidKeyID(id) {
			return "", "", nil, errors.New("WEBHOOK_SECRET_KEYS entries must be id=hex64 pairs")
		}
		if id == activeID {
			return "", "", nil, errors.New("WEBHOOK_SECRET_KEYS must not repeat WEBHOOK_SECRET_KEY_ID")
		}
		if err := validateWebhookSecretKey(key); err != nil {
			return "", "", nil, fmt.Errorf("WEBHOOK_SECRET_KEYS[%s]: %w", id, err)
		}
		previous[id] = key
	}
	return activeID, active, previous, nil
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
