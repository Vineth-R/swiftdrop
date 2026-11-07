// Package secretbox encrypts small secrets (such as webhook signing secrets)
// so they can be stored in the database and recovered later by the worker.
//
// It uses AES-256-GCM: every value gets a fresh random nonce, and any change
// to the stored text or use of the wrong key is detected when decrypting.
// Output format: "v1:" + base64url(nonce || ciphertext).
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

	prefix = "v1:"

	// SecretPrefix starts every generated webhook signing secret.
	SecretPrefix = "whsec_"

	secretRandomBytes = 32
)

var (
	ErrInvalidKey  = errors.New("secretbox: key must be 32 bytes (64 hex characters)")
	ErrInvalidData = errors.New("secretbox: stored value is not valid")
	ErrDecrypt     = errors.New("secretbox: could not decrypt value")
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
	nonce := make([]byte, b.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := b.aead.Seal(nonce, nonce, []byte(plaintext), nil)
	return prefix + base64.RawURLEncoding.EncodeToString(sealed), nil
}

// Decrypt reverses Encrypt. Errors never include the stored value.
func (b *Box) Decrypt(stored string) (string, error) {
	if !strings.HasPrefix(stored, prefix) {
		return "", ErrInvalidData
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(stored, prefix))
	if err != nil {
		return "", ErrInvalidData
	}
	size := b.aead.NonceSize()
	if len(raw) < size+b.aead.Overhead() {
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
