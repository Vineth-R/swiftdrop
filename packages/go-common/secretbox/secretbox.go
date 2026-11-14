// Package secretbox encrypts small secrets (such as webhook signing secrets)
// so they can be stored in the database and recovered later by the worker.
//
// It uses AES-256-GCM: every value gets a fresh random nonce, and any change
// to the stored text or use of the wrong key is detected when decrypting.
// Box writes the legacy format "v1:" + base64url(nonce || ciphertext).
// Keyring writes "v2:<keyId>:" + base64url(nonce || ciphertext).
//
// # Storage
//
// merchant.merchant_webhooks.signing_secret_enc is a BYTEA column. Store the
// UTF-8 bytes of the string returned by Encrypt or EncryptWithAAD,
// []byte(stored), and convert back with string(columnValue) before calling
// Decrypt or DecryptWithAAD. The text format is the canonical stored
// representation; no second encoding is applied.
//
// # Key rotation
//
// Box uses one key and writes the legacy "v1:" format. New code should use
// Keyring, which supports a named active key and decryption-only previous
// keys: ciphertext is "v2:<keyId>:<base64url(nonce || ciphertext)>". During a
// rotation, keep the old key available for decryption until every stored value
// has been re-encrypted with the active key. Legacy "v1:" values are decrypted
// by trying each configured key.
//
// # Additional authenticated data
//
// Keyring.EncryptWithAAD and Keyring.DecryptWithAAD bind a ciphertext to a
// caller-supplied context (for a webhook secret, "merchant.webhook:<id>").
// Decrypting a value under a different context fails authentication, so a
// ciphertext copied to another row does not unlock. Legacy "v1:" values carry
// no AAD and remain readable.
package secretbox

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
)

const (
	// KeySize is the required key length in bytes (AES-256).
	KeySize = 32

	// MaxPlaintextBytes bounds the secrets this package accepts. It is meant
	// for small values such as a webhook signing secret (a generated secret is
	// 70 bytes); 4 KiB leaves generous headroom while keeping Decrypt from
	// allocating memory proportional to an arbitrarily large stored value.
	MaxPlaintextBytes = 4096

	prefix = "v1:"

	// SecretPrefix starts every generated webhook signing secret.
	SecretPrefix = "whsec_"

	secretRandomBytes = 32
)

var (
	ErrInvalidKey        = errors.New("secretbox: key must be 32 bytes (64 hex characters)")
	ErrInvalidData       = errors.New("secretbox: stored value is not valid")
	ErrDecrypt           = errors.New("secretbox: could not decrypt value")
	ErrPlaintextTooLarge = errors.New("secretbox: plaintext is too large")
)

// Box encrypts and decrypts values with one key. It is safe for concurrent use.
type Box struct {
	aead cipher.AEAD
}

// New builds a Box from a 32-byte key.
func New(key []byte) (*Box, error) {
	if len(key) != KeySize {
		return nil, ErrInvalidKey
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		// aes.NewCipher only rejects unsupported key lengths, which the check
		// above already rules out.
		return nil, ErrInvalidKey
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, ErrInvalidKey
	}
	return &Box{aead: aead}, nil
}

// NewFromHex builds a Box from a 64-character hex key, the format used in
// the WEBHOOK_SECRET_KEY environment variable.
func NewFromHex(hexKey string) (*Box, error) {
	key, err := hex.DecodeString(strings.TrimSpace(hexKey))
	if err != nil {
		return nil, ErrInvalidKey
	}
	return New(key)
}

// Encrypt returns the storable form of plaintext.
func (b *Box) Encrypt(plaintext string) (string, error) {
	if len(plaintext) > MaxPlaintextBytes {
		return "", ErrPlaintextTooLarge
	}

	nonce := make([]byte, b.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}

	// Keep the nonce and the authenticated ciphertext together. Building the
	// output explicitly avoids relying on Seal's destination-slice behaviour.
	ciphertext := b.aead.Seal(nil, nonce, []byte(plaintext), nil)
	stored := make([]byte, 0, len(nonce)+len(ciphertext))
	stored = append(stored, nonce...)
	stored = append(stored, ciphertext...)

	return prefix + base64.RawURLEncoding.EncodeToString(stored), nil
}

// Decrypt reverses Encrypt. Errors never include the stored value.
func (b *Box) Decrypt(stored string) (string, error) {
	if !strings.HasPrefix(stored, prefix) {
		return "", ErrInvalidData
	}
	encoded := strings.TrimPrefix(stored, prefix)

	// Reject an oversized value before decoding so a corrupted or hostile
	// stored value cannot force a large allocation. Encrypt can never produce
	// a value that exceeds this bound.
	maxRaw := b.aead.NonceSize() + MaxPlaintextBytes + b.aead.Overhead()
	if len(encoded) > base64.RawURLEncoding.EncodedLen(maxRaw) {
		return "", ErrInvalidData
	}

	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return "", ErrInvalidData
	}

	size := b.aead.NonceSize()
	if len(raw) < size+b.aead.Overhead() || len(raw) > maxRaw {
		return "", ErrInvalidData
	}

	plain, err := b.aead.Open(nil, raw[:size], raw[size:], nil)
	if err != nil {
		return "", ErrDecrypt
	}
	return string(plain), nil
}

// GenerateSecret returns a new random webhook signing secret: "whsec_" plus
// 64 hex characters (256 bits).
func GenerateSecret() (string, error) {
	buf := make([]byte, secretRandomBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return SecretPrefix + hex.EncodeToString(buf), nil
}
