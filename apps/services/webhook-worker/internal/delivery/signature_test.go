package delivery

import "testing"

// The expected value was computed outside Go, with Python's hmac module, so
// this test would catch a mistake in how Sign builds "<timestamp>.<body>".
func TestSignKnownVector(t *testing.T) {
	const want = "sha256=37f671ba221c8595e0b5e040b978c8f8d411d0578f63858c7289eb8b033bb2cc"
	got := Sign("whsec_test_secret", 1762000000, []byte(`{"a":1}`))
	if got != want {
		t.Fatalf("Sign() = %q, want %q", got, want)
	}
}

func TestVerify(t *testing.T) {
	const secret = "whsec_test_secret"
	body := []byte(`{"a":1}`)
	good := Sign(secret, 1762000000, body)

	tests := []struct {
		name      string
		secret    string
		timestamp int64
		body      []byte
		signature string
		want      bool
	}{
		{"correct", secret, 1762000000, body, good, true},
		{"wrong secret", "other", 1762000000, body, good, false},
		{"changed body", secret, 1762000000, []byte(`{"a":2}`), good, false},
		{"changed timestamp", secret, 1762000001, body, good, false},
		{"missing prefix", secret, 1762000000, body, good[len("sha256="):], false},
		{"empty signature", secret, 1762000000, body, "", false},
		{"truncated signature", secret, 1762000000, body, good[:len(good)-2], false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Verify(tc.secret, tc.timestamp, tc.body, tc.signature); got != tc.want {
				t.Fatalf("Verify() = %v, want %v", got, tc.want)
			}
		})
	}
}
