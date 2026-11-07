package secretbox

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"
)

const testKeyHex = "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f"

func newTestBox(t *testing.T) *Box {
	t.Helper()
	box, err := NewFromHex(testKeyHex)
	if err != nil {
		t.Fatalf("NewFromHex: %v", err)
	}
	return box
}

func TestRoundTrip(t *testing.T) {
	box := newTestBox(t)
	for _, plain := range []string{"whsec_abc123", "", "unicode ✓ secret"} {
		stored, err := box.Encrypt(plain)
		if err != nil {
			t.Fatalf("Encrypt(%q): %v", plain, err)
		}
		got, err := box.Decrypt(stored)
		if err != nil {
			t.Fatalf("Decrypt: %v", err)
		}
		if got != plain {
			t.Fatalf("round trip = %q, want %q", got, plain)
		}
	}
}

func TestEncryptIsRandomised(t *testing.T) {
	box := newTestBox(t)
	a, _ := box.Encrypt("same")
	b, _ := box.Encrypt("same")
	if a == b {
		t.Fatal("two encryptions of the same value must differ")
	}
}

func TestStoredValueHidesPlaintext(t *testing.T) {
	stored, _ := newTestBox(t).Encrypt("whsec_visible")
	if strings.Contains(stored, "whsec_visible") || !strings.HasPrefix(stored, "v1:") {
		t.Fatalf("unexpected stored form %q", stored)
	}
}

func TestWrongKeyFails(t *testing.T) {
	stored, _ := newTestBox(t).Encrypt("secret")
	other, err := New(make([]byte, KeySize))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := other.Decrypt(stored); !errors.Is(err, ErrDecrypt) {
		t.Fatalf("err = %v, want ErrDecrypt", err)
	}
}

func TestTamperedValueFails(t *testing.T) {
	box := newTestBox(t)
	stored, _ := box.Encrypt("secret")

	// Flip one bit of a real byte in the decoded data. (Changing the last
	// base64 character is not reliable: it can hold unused padding bits.)
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(stored, "v1:"))
	if err != nil {
		t.Fatal(err)
	}
	raw[len(raw)-1] ^= 0x01
	tampered := "v1:" + base64.RawURLEncoding.EncodeToString(raw)

	if _, err := box.Decrypt(tampered); !errors.Is(err, ErrDecrypt) {
		t.Fatalf("err = %v, want ErrDecrypt", err)
	}
}

func TestInvalidStoredValues(t *testing.T) {
	box := newTestBox(t)
	for _, bad := range []string{"", "plain", "v2:abc", "v1:!!!notbase64", "v1:AAAA"} {
		if _, err := box.Decrypt(bad); !errors.Is(err, ErrInvalidData) {
			t.Errorf("Decrypt(%q) err = %v, want ErrInvalidData", bad, err)
		}
	}
}

func TestKeyValidation(t *testing.T) {
	if _, err := New(make([]byte, 16)); !errors.Is(err, ErrInvalidKey) {
		t.Fatalf("short key err = %v", err)
	}
	for _, bad := range []string{"", "zz", "abcd"} {
		if _, err := NewFromHex(bad); !errors.Is(err, ErrInvalidKey) {
			t.Errorf("NewFromHex(%q) err = %v", bad, err)
		}
	}
	if _, err := NewFromHex(" " + testKeyHex + "\n"); err != nil {
		t.Fatalf("surrounding whitespace should be trimmed: %v", err)
	}
}

func TestGenerateSecret(t *testing.T) {
	a, err := GenerateSecret()
	if err != nil {
		t.Fatal(err)
	}
	b, _ := GenerateSecret()
	if !strings.HasPrefix(a, SecretPrefix) || len(a) != len(SecretPrefix)+64 {
		t.Fatalf("unexpected secret shape %q", a)
	}
	if a == b {
		t.Fatal("generated secrets must differ")
	}
}
