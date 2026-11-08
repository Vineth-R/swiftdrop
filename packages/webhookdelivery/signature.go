// Package delivery sends one signed webhook request to a merchant and decides
// when a failed delivery is retried. It has no RabbitMQ or database code, so
// every rule in API v1.1 section 10 can be tested with a fake merchant server.
package webhookdelivery

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
	"time"
)

// signaturePrefix is the scheme part of the X-SwiftDrop-Signature header.
const signaturePrefix = "sha256="

// SignatureMaxAge is the acceptance window for a request timestamp (API v1.1,
// section 10.2). A request is rejected when its timestamp is more than five
// minutes in the past, and also when it is more than five minutes in the
// future so a captured request cannot widen its own replay window.
const SignatureMaxAge = 5 * time.Minute

// Sign returns the X-SwiftDrop-Signature header value: "sha256=" followed by
// the hex HMAC-SHA256 of "<timestamp>.<raw body>" keyed with the webhook
// signing secret (API v1.1, section 10.1). The timestamp is Unix seconds and
// is the same value sent in X-SwiftDrop-Timestamp. Signing the timestamp as
// well as the body stops an old, captured request from being replayed later.
func Sign(secret string, timestamp int64, body []byte) string {
	return signTimestamp(secret, strconv.FormatInt(timestamp, 10), body)
}

// signTimestamp signs the exact timestamp text that travels in the header, so
// a non-canonical value cannot be swapped for a canonical one.
func signTimestamp(secret, timestamp string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp))
	mac.Write([]byte("."))
	mac.Write(body)
	return signaturePrefix + hex.EncodeToString(mac.Sum(nil))
}

// Verify reports whether a webhook request is authentic and fresh. It is the
// complete merchant-side check from API v1.1 section 10.2: it parses the
// X-SwiftDrop-Timestamp header value, rejects timestamps outside
// SignatureMaxAge, and compares the signature in constant time. It uses the
// current time; use VerifyAt to supply it explicitly (for tests or when the
// receiver's clock is not the system clock).
func Verify(secret, timestamp string, body []byte, signature string) bool {
	return VerifyAt(secret, timestamp, body, signature, time.Now())
}

// VerifyAt is Verify with an explicit current time. The timestamp is the raw
// X-SwiftDrop-Timestamp header value, not a parsed integer, so the HMAC is
// computed over the exact bytes the sender signed. Malformed, non-canonical,
// non-positive, stale and future-dated timestamps are rejected.
func VerifyAt(secret, timestamp string, body []byte, signature string, now time.Time) bool {
	if !strings.HasPrefix(signature, signaturePrefix) {
		return false
	}
	ts, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil || ts <= 0 {
		return false
	}
	// Require canonical Unix seconds ("1762000000", not "01762000000" or
	// "+1762000000"). Signing the raw text alone would accept those forms when
	// a caller signed the same non-canonical string.
	if strconv.FormatInt(ts, 10) != timestamp {
		return false
	}
	if !withinReplayWindow(ts, now) {
		return false
	}
	want := signTimestamp(secret, timestamp, body)
	return hmac.Equal([]byte(want), []byte(signature))
}

// withinReplayWindow reports whether timestamp is within SignatureMaxAge of
// now in either direction.
func withinReplayWindow(timestamp int64, now time.Time) bool {
	window := int64(SignatureMaxAge / time.Second)
	delta := now.Unix() - timestamp
	return delta >= -window && delta <= window
}
