// Package delivery sends one signed webhook request to a merchant and decides
// when a failed delivery is retried. It has no RabbitMQ or database code, so
// every rule in API v1.1 section 10 can be tested with a fake merchant server.
package delivery

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
)

// signaturePrefix is the scheme part of the X-SwiftDrop-Signature header.
const signaturePrefix = "sha256="

// Sign returns the X-SwiftDrop-Signature header value: "sha256=" followed by
// the hex HMAC-SHA256 of "<timestamp>.<raw body>" keyed with the webhook
// signing secret (API v1.1, section 10.1). The timestamp is Unix seconds and
// is the same value sent in X-SwiftDrop-Timestamp. Signing the timestamp as
// well as the body stops an old, captured request from being replayed later.
func Sign(secret string, timestamp int64, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(strconv.FormatInt(timestamp, 10)))
	mac.Write([]byte("."))
	mac.Write(body)
	return signaturePrefix + hex.EncodeToString(mac.Sum(nil))
}

// Verify reports whether signature is the correct signature for the secret,
// timestamp and raw body. It compares in constant time. SwiftDrop never
// verifies its own requests; this is what a merchant does, and the tests and
// the merchant documentation use it as the reference.
func Verify(secret string, timestamp int64, body []byte, signature string) bool {
	if !strings.HasPrefix(signature, signaturePrefix) {
		return false
	}
	want := Sign(secret, timestamp, body)
	return hmac.Equal([]byte(want), []byte(signature))
}
