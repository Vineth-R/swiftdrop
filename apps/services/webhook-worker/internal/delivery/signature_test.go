package delivery

import (
	"strconv"
	"testing"
	"time"
)

// The expected value was computed outside Go, with Python's hmac module, so
// this test would catch a mistake in how Sign builds "<timestamp>.<body>".
func TestSignKnownVector(t *testing.T) {
	const want = "sha256=37f671ba221c8595e0b5e040b978c8f8d411d0578f63858c7289eb8b033bb2cc"
	got := Sign("whsec_test_secret", 1762000000, []byte(`{"a":1}`))
	if got != want {
		t.Fatalf("Sign() = %q, want %q", got, want)
	}
}

func TestVerifyAt(t *testing.T) {
	const (
		secret = "whsec_test_secret"
		tsText = "1762000000"
		window = SignatureMaxAge
	)
	base := time.Unix(1762000000, 0)
	body := []byte(`{"a":1}`)
	good := Sign(secret, 1762000000, body)

	tests := []struct {
		name      string
		secret    string
		timestamp string
		now       time.Time
		body      []byte
		signature string
		want      bool
	}{
		{"correct", secret, tsText, base, body, good, true},
		{"wrong secret", "other", tsText, base, body, good, false},
		{"changed body", secret, tsText, base, []byte(`{"a":2}`), good, false},
		{"changed timestamp", secret, "1762000001", base, body, good, false},
		{"missing prefix", secret, tsText, base, body, good[len("sha256="):], false},
		{"empty signature", secret, tsText, base, body, "", false},
		{"truncated signature", secret, tsText, base, body, good[:len(good)-2], false},
		{"exactly five minutes old", secret, tsText, base.Add(window), body, good, true},
		{"older than five minutes", secret, tsText, base.Add(window + time.Second), body, good, false},
		{"exactly five minutes in the future", secret, tsText, base.Add(-window), body, good, true},
		{"more than five minutes in the future", secret, tsText, base.Add(-window - time.Second), body, good, false},
		{"empty timestamp", secret, "", base, body, good, false},
		{"non-numeric timestamp", secret, "not-a-number", base, body, good, false},
		{"fractional timestamp", secret, "1762000000.5", base, body, good, false},
		{"timestamp with whitespace", secret, " 1762000000", base, body, good, false},
		{"zero timestamp", secret, "0", base, body, Sign(secret, 0, body), false},
		{"negative timestamp", secret, "-1", base, body, Sign(secret, -1, body), false},
		{"non-canonical timestamp", secret, "01762000000", base, body, good, false},
		{"timestamp beyond int64", secret, "999999999999999999999999", base, body, good, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := VerifyAt(tc.secret, tc.timestamp, tc.body, tc.signature, tc.now); got != tc.want {
				t.Fatalf("VerifyAt() = %v, want %v", got, tc.want)
			}
		})
	}
}

// A timestamp must be canonical Unix seconds. A non-canonical string is
// rejected even when the supplied signature was computed over that exact text,
// so verification cannot depend on how the sender happened to encode it.
func TestVerifyRejectsNonCanonicalTimestamp(t *testing.T) {
	const secret = "whsec_test_secret"
	body := []byte(`{"a":1}`)
	now := time.Unix(1762000000, 0)
	for _, ts := range []string{"01762000000", "001762000000", "+1762000000"} {
		sig := signTimestamp(secret, ts, body)
		if VerifyAt(secret, ts, body, sig, now) {
			t.Errorf("non-canonical timestamp %q was accepted", ts)
		}
	}
}

// Verify is the convenience form that uses the current time. A freshly signed
// timestamp must pass and a stale one must fail.
func TestVerifyUsesCurrentTime(t *testing.T) {
	const secret = "whsec_test_secret"
	body := []byte(`{"a":1}`)

	now := time.Now().Unix()
	fresh := strconv.FormatInt(now, 10)
	if !Verify(secret, fresh, body, Sign(secret, now, body)) {
		t.Fatal("a freshly signed request did not verify")
	}

	stale := now - int64((SignatureMaxAge+time.Minute)/time.Second)
	staleText := strconv.FormatInt(stale, 10)
	if Verify(secret, staleText, body, Sign(secret, stale, body)) {
		t.Fatal("a stale request verified")
	}
}
