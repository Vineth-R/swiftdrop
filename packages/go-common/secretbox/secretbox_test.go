package secretbox

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"sync"
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

func mustEncrypt(t *testing.T, box *Box, plain string) string {
	t.Helper()
	stored, err := box.Encrypt(plain)
	if err != nil {
		t.Fatalf("Encrypt(%q): %v", plain, err)
	}
	return stored
}

func decodeStored(t *testing.T, stored string) []byte {
	t.Helper()
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(stored, prefix))
	if err != nil {
		t.Fatalf("decode stored value: %v", err)
	}
	return raw
}

func encodeStored(raw []byte) string {
	return prefix + base64.RawURLEncoding.EncodeToString(raw)
}

func TestRoundTrip(t *testing.T) {
	box := newTestBox(t)
	for _, plain := range []string{"whsec_abc123", "", "unicode ✓ secret", "line\nbreak"} {
		stored := mustEncrypt(t, box, plain)
		got, err := box.Decrypt(stored)
		if err != nil {
			t.Fatalf("Decrypt(%q): %v", plain, err)
		}
		if got != plain {
			t.Fatalf("round trip = %q, want %q", got, plain)
		}
	}
}

func TestEncryptIsRandomised(t *testing.T) {
	box := newTestBox(t)
	a := mustEncrypt(t, box, "same")
	b := mustEncrypt(t, box, "same")
	if a == b {
		t.Fatal("two encryptions of the same value must differ")
	}
}

func TestStoredValueHidesPlaintext(t *testing.T) {
	stored := mustEncrypt(t, newTestBox(t), "whsec_visible")
	if strings.Contains(stored, "whsec_visible") {
		t.Fatalf("stored value contains the plaintext: %q", stored)
	}
	if !strings.HasPrefix(stored, prefix) {
		t.Fatalf("stored value = %q, want prefix %q", stored, prefix)
	}
}

func TestStoredValueSurvivesByteaConversion(t *testing.T) {
	box := newTestBox(t)
	stored := mustEncrypt(t, box, "whsec_bytea")

	// signing_secret_enc is BYTEA. The documented persistence step stores the
	// UTF-8 bytes of the textual form and converts back to a string on read.
	got, err := box.Decrypt(string([]byte(stored)))
	if err != nil {
		t.Fatalf("Decrypt after BYTEA round trip: %v", err)
	}
	if got != "whsec_bytea" {
		t.Fatalf("round trip = %q, want whsec_bytea", got)
	}
}

func TestWrongKeyFails(t *testing.T) {
	stored := mustEncrypt(t, newTestBox(t), "secret")
	other, err := New(make([]byte, KeySize))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := other.Decrypt(stored); !errors.Is(err, ErrDecrypt) {
		t.Fatalf("err = %v, want ErrDecrypt", err)
	}
}

func TestTamperedCiphertextFails(t *testing.T) {
	box := newTestBox(t)
	stored := mustEncrypt(t, box, "secret")

	// Flip one bit of a real byte in the decoded data. (Changing the last
	// base64 character is not reliable: it can hold unused padding bits.)
	raw := decodeStored(t, stored)
	raw[len(raw)-1] ^= 0x01

	if _, err := box.Decrypt(encodeStored(raw)); !errors.Is(err, ErrDecrypt) {
		t.Fatalf("err = %v, want ErrDecrypt", err)
	}
}

func TestTamperedNonceFails(t *testing.T) {
	box := newTestBox(t)
	stored := mustEncrypt(t, box, "secret")

	raw := decodeStored(t, stored)
	raw[0] ^= 0x01

	if _, err := box.Decrypt(encodeStored(raw)); !errors.Is(err, ErrDecrypt) {
		t.Fatalf("err = %v, want ErrDecrypt", err)
	}
}

func TestTruncatedValueFails(t *testing.T) {
	box := newTestBox(t)
	raw := decodeStored(t, mustEncrypt(t, box, "a secret value"))
	size := box.aead.NonceSize()

	tests := []struct {
		name string
		raw  []byte
		want error
	}{
		{name: "nonce only", raw: raw[:size], want: ErrInvalidData},
		{name: "one byte short of nonce and tag", raw: raw[:size+box.aead.Overhead()-1], want: ErrInvalidData},
		{name: "nonce and tag but no ciphertext", raw: raw[:size+box.aead.Overhead()], want: ErrDecrypt},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := box.Decrypt(encodeStored(test.raw)); !errors.Is(err, test.want) {
				t.Fatalf("err = %v, want %v", err, test.want)
			}
		})
	}
}

func TestInvalidStoredValues(t *testing.T) {
	box := newTestBox(t)
	nonceOnly := make([]byte, box.aead.NonceSize())
	for _, bad := range []string{
		"",
		"plain",
		"v2:abc",
		"v1:",
		"v1:!!!notbase64",
		"v1:AAAA",
		"v1:AAAA====",
		encodeStored(nonceOnly),
		" v1:AAAA",
	} {
		if _, err := box.Decrypt(bad); !errors.Is(err, ErrInvalidData) {
			t.Errorf("Decrypt(%q) err = %v, want ErrInvalidData", bad, err)
		}
	}
}

func TestOversizedStoredValueFails(t *testing.T) {
	box := newTestBox(t)
	maxRaw := box.aead.NonceSize() + MaxPlaintextBytes + box.aead.Overhead()
	oversized := prefix + strings.Repeat("A", base64.RawURLEncoding.EncodedLen(maxRaw)+4)

	if _, err := box.Decrypt(oversized); !errors.Is(err, ErrInvalidData) {
		t.Fatalf("err = %v, want ErrInvalidData", err)
	}
}

func TestPlaintextLimit(t *testing.T) {
	box := newTestBox(t)

	if _, err := box.Encrypt(strings.Repeat("a", MaxPlaintextBytes+1)); !errors.Is(err, ErrPlaintextTooLarge) {
		t.Fatalf("oversized plaintext err = %v, want ErrPlaintextTooLarge", err)
	}

	plain := strings.Repeat("a", MaxPlaintextBytes)
	got, err := box.Decrypt(mustEncrypt(t, box, plain))
	if err != nil {
		t.Fatalf("Decrypt at the limit: %v", err)
	}
	if got != plain {
		t.Fatal("value at the plaintext limit did not round trip")
	}
}

func TestKeyValidation(t *testing.T) {
	for _, key := range [][]byte{make([]byte, 0), make([]byte, KeySize-1), make([]byte, KeySize+1)} {
		if _, err := New(key); !errors.Is(err, ErrInvalidKey) {
			t.Errorf("New(%d bytes) err = %v, want ErrInvalidKey", len(key), err)
		}
	}

	for _, bad := range []string{
		"",
		"zz",
		"abcd",
		strings.Repeat("0", 62),
		strings.Repeat("0", 66),
		" " + strings.Repeat("0", 63) + "\n",
	} {
		if _, err := NewFromHex(bad); !errors.Is(err, ErrInvalidKey) {
			t.Errorf("NewFromHex(%q) err = %v, want ErrInvalidKey", bad, err)
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
	b, err := GenerateSecret()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(a, SecretPrefix) || len(a) != len(SecretPrefix)+64 {
		t.Fatalf("unexpected secret shape %q", a)
	}
	if a == b {
		t.Fatal("generated secrets must differ")
	}
}

func TestConcurrentUse(t *testing.T) {
	box := newTestBox(t)

	const goroutines = 16
	const iterations = 25

	var wg sync.WaitGroup
	errs := make(chan error, goroutines*iterations)
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				plain := fmt.Sprintf("whsec_%d_%d", g, i)
				stored, err := box.Encrypt(plain)
				if err != nil {
					errs <- fmt.Errorf("Encrypt: %w", err)
					return
				}
				got, err := box.Decrypt(stored)
				if err != nil {
					errs <- fmt.Errorf("Decrypt: %w", err)
					return
				}
				if got != plain {
					errs <- fmt.Errorf("round trip = %q, want %q", got, plain)
					return
				}
			}
		}(g)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}

// FuzzDecrypt checks that arbitrary stored input is rejected without a panic.
// A value that does decrypt must round trip through Encrypt again.
func FuzzDecrypt(f *testing.F) {
	box, err := NewFromHex(testKeyHex)
	if err != nil {
		f.Fatalf("NewFromHex: %v", err)
	}
	valid, err := box.Encrypt("whsec_fuzz_seed")
	if err != nil {
		f.Fatalf("Encrypt: %v", err)
	}
	for _, seed := range []string{"", "v1:", "v1:AAAA", "v2:abc", valid} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, stored string) {
		plain, err := box.Decrypt(stored)
		if err != nil {
			return
		}
		again, err := box.Encrypt(plain)
		if err != nil {
			t.Fatalf("Encrypt decrypted value: %v", err)
		}
		got, err := box.Decrypt(again)
		if err != nil {
			t.Fatalf("Decrypt re-encrypted value: %v", err)
		}
		if got != plain {
			t.Fatalf("round trip = %q, want %q", got, plain)
		}
	})
}
