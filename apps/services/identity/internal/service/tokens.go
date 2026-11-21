package service

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
)

const (
	refreshTokenPrefix = "rt_"
	refreshTokenBytes  = 32

	// maxRefreshTokenLength bounds the work done on untrusted input.
	maxRefreshTokenLength = 256
)

// newRefreshToken returns a random refresh token and its SHA-256 hash. Only the
// hash is stored (DB v1.1 section 3.4).
func newRefreshToken() (token, hash string, err error) {
	var buffer [refreshTokenBytes]byte
	if _, err := rand.Read(buffer[:]); err != nil {
		return "", "", fmt.Errorf("generate refresh token: %w", err)
	}
	token = refreshTokenPrefix + base64.RawURLEncoding.EncodeToString(buffer[:])
	return token, hashRefreshToken(token), nil
}

func hashRefreshToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// plausibleRefreshToken rejects obviously malformed input before it reaches the
// database.
func plausibleRefreshToken(token string) bool {
	return strings.HasPrefix(token, refreshTokenPrefix) && len(token) <= maxRefreshTokenLength
}
