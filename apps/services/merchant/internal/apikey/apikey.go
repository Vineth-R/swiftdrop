package apikey

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

const (
	Prefix       = "sdk_live_"
	LookupBytes  = 8
	SecretBytes  = 24
	LookupLength = LookupBytes * 2
)

var ErrInvalid = errors.New("invalid merchant API key")

type Key struct {
	Full      string
	LookupID  string
	KeyPrefix string
	Hash      string
}

func Generate() (Key, error) {
	lookup := make([]byte, LookupBytes)
	secret := make([]byte, SecretBytes)
	if _, err := rand.Read(lookup); err != nil {
		return Key{}, fmt.Errorf("generate API key lookup: %w", err)
	}
	if _, err := rand.Read(secret); err != nil {
		return Key{}, fmt.Errorf("generate API key secret: %w", err)
	}
	lookupID := hex.EncodeToString(lookup)
	full := Prefix + lookupID + "_" + hex.EncodeToString(secret)
	return Key{Full: full, LookupID: lookupID, KeyPrefix: Prefix + lookupID[:8], Hash: Hash(full)}, nil
}

func Parse(full string) (lookupID string, err error) {
	full = strings.TrimSpace(full)
	if !strings.HasPrefix(full, Prefix) {
		return "", ErrInvalid
	}
	remainder := strings.TrimPrefix(full, Prefix)
	parts := strings.Split(remainder, "_")
	if len(parts) != 2 || len(parts[0]) != LookupLength || len(parts[1]) != SecretBytes*2 {
		return "", ErrInvalid
	}
	if _, err := hex.DecodeString(parts[0]); err != nil {
		return "", ErrInvalid
	}
	if _, err := hex.DecodeString(parts[1]); err != nil {
		return "", ErrInvalid
	}
	return parts[0], nil
}

func Hash(full string) string {
	digest := sha256.Sum256([]byte(full))
	return hex.EncodeToString(digest[:])
}

func EqualHash(stored, supplied string) bool {
	if len(stored) != len(supplied) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(strings.ToLower(stored)), []byte(strings.ToLower(supplied))) == 1
}
