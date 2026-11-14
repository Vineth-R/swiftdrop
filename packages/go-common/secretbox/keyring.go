package secretbox

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"regexp"
	"sort"
	"strings"
)

// ErrInvalidKeyring means the configured key set is empty, malformed, or
// contains a duplicate or invalid key identifier.
var ErrInvalidKeyring = errors.New("secretbox: keyring configuration is invalid")

var keyIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,32}$`)

// ValidKeyID reports whether id is a usable key identifier.
func ValidKeyID(id string) bool { return keyIDPattern.MatchString(id) }

// Key is one named encryption key.
type Key struct {
	ID  string
	Key []byte
}

// Keyring encrypts with one active key and can still decrypt every value
// written by a key that remains configured. Ciphertext is self-describing:
//
//	v2:<keyId>:<base64url(nonce || ciphertext)>
//
// Legacy "v1:" values have no key identifier; they are decrypted with each
// configured key in turn and accepted when one authenticates. New writes never
// use the legacy format. Callers can read-repair a legacy value by decrypting
// it and re-encrypting the same plaintext with EncryptWithAAD; a rotation only
// needs the old key kept in the keyring until every stored row has been
// re-encrypted.
type Keyring struct {
	activeID   string
	active     cipher.AEAD
	decryptors map[string]cipher.AEAD
	ids        []string
}

// NewKeyring builds a keyring from named keys. activeID must name one of keys.
func NewKeyring(activeID string, keys []Key) (*Keyring, error) {
	if !keyIDPattern.MatchString(activeID) {
		return nil, ErrInvalidKeyring
	}
	ring := &Keyring{
		activeID:   activeID,
		decryptors: make(map[string]cipher.AEAD, len(keys)),
	}
	for _, key := range keys {
		if !keyIDPattern.MatchString(key.ID) {
			return nil, ErrInvalidKeyring
		}
		if _, exists := ring.decryptors[key.ID]; exists {
			return nil, ErrInvalidKeyring
		}
		aead, err := newAEAD(key.Key)
		if err != nil {
			return nil, ErrInvalidKeyring
		}
		ring.decryptors[key.ID] = aead
		if key.ID == activeID {
			ring.active = aead
		}
		ring.ids = append(ring.ids, key.ID)
	}
	if ring.active == nil {
		return nil, ErrInvalidKeyring
	}
	sort.Strings(ring.ids)
	return ring, nil
}

// NewKeyringFromHex builds a keyring from hexadecimal keys. previous maps a key
// identifier to a 64-character hex key and is used only for decryption.
func NewKeyringFromHex(activeID, activeHex string, previous map[string]string) (*Keyring, error) {
	if strings.TrimSpace(activeID) == "" {
		activeID = "k1"
	}
	activeKey, err := hex.DecodeString(strings.TrimSpace(activeHex))
	if err != nil {
		return nil, ErrInvalidKeyring
	}
	keys := []Key{{ID: activeID, Key: activeKey}}
	for id, encoded := range previous {
		if id == activeID {
			continue
		}
		decoded, err := hex.DecodeString(strings.TrimSpace(encoded))
		if err != nil {
			return nil, ErrInvalidKeyring
		}
		keys = append(keys, Key{ID: id, Key: decoded})
	}
	return NewKeyring(activeID, keys)
}

// ActiveKeyID returns the identifier used for new ciphertext.
func (k *Keyring) ActiveKeyID() string { return k.activeID }

// Encrypt encrypts without additional authenticated data.
func (k *Keyring) Encrypt(plaintext string) (string, error) {
	return k.EncryptWithAAD(plaintext, "")
}

// EncryptWithAAD encrypts and binds the ciphertext to aad. Decryption with a
// different aad fails authentication.
func (k *Keyring) EncryptWithAAD(plaintext, aad string) (string, error) {
	if len(plaintext) > MaxPlaintextBytes {
		return "", ErrPlaintextTooLarge
	}
	nonce := make([]byte, k.active.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := k.active.Seal(nil, nonce, []byte(plaintext), []byte(aad))
	raw := make([]byte, 0, len(nonce)+len(sealed))
	raw = append(raw, nonce...)
	raw = append(raw, sealed...)
	return "v2:" + k.activeID + ":" + base64.RawURLEncoding.EncodeToString(raw), nil
}

// Decrypt decrypts without additional authenticated data.
func (k *Keyring) Decrypt(stored string) (string, error) {
	return k.DecryptWithAAD(stored, "")
}

// DecryptWithAAD reverses EncryptWithAAD. Errors never include the stored
// value or key material.
func (k *Keyring) DecryptWithAAD(stored, aad string) (string, error) {
	if strings.HasPrefix(stored, "v2:") {
		return k.decryptVersioned(stored, aad)
	}
	if strings.HasPrefix(stored, prefix) {
		return k.decryptLegacy(stored)
	}
	return "", ErrInvalidData
}

func (k *Keyring) decryptVersioned(stored, aad string) (string, error) {
	rest := strings.TrimPrefix(stored, "v2:")
	separator := strings.IndexByte(rest, ':')
	if separator <= 0 {
		return "", ErrInvalidData
	}
	keyID := rest[:separator]
	aead, ok := k.decryptors[keyID]
	if !ok {
		// The value was written with a key that is no longer configured. Fail
		// closed rather than trying unrelated keys.
		return "", ErrDecrypt
	}
	return decryptPayload(aead, rest[separator+1:], aad)
}

func (k *Keyring) decryptLegacy(stored string) (string, error) {
	encoded := strings.TrimPrefix(stored, prefix)
	for _, id := range k.ids {
		plain, err := decryptPayload(k.decryptors[id], encoded, "")
		if err == nil {
			return plain, nil
		}
	}
	return "", ErrDecrypt
}

func newAEAD(key []byte) (cipher.AEAD, error) {
	if len(key) != KeySize {
		return nil, ErrInvalidKey
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, ErrInvalidKey
	}
	return cipher.NewGCM(block)
}

func decryptPayload(aead cipher.AEAD, encoded, aad string) (string, error) {
	maxRaw := aead.NonceSize() + MaxPlaintextBytes + aead.Overhead()
	if len(encoded) > base64.RawURLEncoding.EncodedLen(maxRaw) {
		return "", ErrInvalidData
	}
	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return "", ErrInvalidData
	}
	size := aead.NonceSize()
	if len(raw) < size+aead.Overhead() || len(raw) > maxRaw {
		return "", ErrInvalidData
	}
	plain, err := aead.Open(nil, raw[:size], raw[size:], []byte(aad))
	if err != nil {
		return "", ErrDecrypt
	}
	return string(plain), nil
}
