package secretbox

import (
	"errors"
	"strings"
	"testing"
)

func testKey(seed byte) []byte {
	key := make([]byte, KeySize)
	for i := range key {
		key[i] = seed + byte(i)
	}
	return key
}

func TestKeyringRoundTripWithAAD(t *testing.T) {
	ring, err := NewKeyring("k1", []Key{{ID: "k1", Key: testKey(1)}})
	if err != nil {
		t.Fatalf("NewKeyring() = %v", err)
	}
	stored, err := ring.EncryptWithAAD("whsec_secret", "merchant.webhook:merchant-1")
	if err != nil {
		t.Fatalf("EncryptWithAAD() = %v", err)
	}
	if !strings.HasPrefix(stored, "v2:k1:") {
		t.Fatalf("stored value = %q, want v2:k1 prefix", stored)
	}
	plain, err := ring.DecryptWithAAD(stored, "merchant.webhook:merchant-1")
	if err != nil || plain != "whsec_secret" {
		t.Fatalf("DecryptWithAAD() = %q, %v", plain, err)
	}
}

func TestKeyringRejectsWrongAAD(t *testing.T) {
	ring, _ := NewKeyring("k1", []Key{{ID: "k1", Key: testKey(1)}})
	stored, err := ring.EncryptWithAAD("whsec_secret", "merchant.webhook:merchant-1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ring.DecryptWithAAD(stored, "merchant.webhook:merchant-2"); !errors.Is(err, ErrDecrypt) {
		t.Fatalf("DecryptWithAAD() error = %v, want ErrDecrypt", err)
	}
}

func TestKeyringRotationKeepsOldValuesReadable(t *testing.T) {
	oldRing, err := NewKeyring("k1", []Key{{ID: "k1", Key: testKey(1)}})
	if err != nil {
		t.Fatal(err)
	}
	stored, err := oldRing.EncryptWithAAD("old-secret", "merchant.webhook:merchant-1")
	if err != nil {
		t.Fatal(err)
	}

	rotated, err := NewKeyring("k2", []Key{
		{ID: "k2", Key: testKey(2)},
		{ID: "k1", Key: testKey(1)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rotated.DecryptWithAAD(stored, "merchant.webhook:merchant-1"); err != nil {
		t.Fatalf("old ciphertext should decrypt after rotation: %v", err)
	}

	newStored, err := rotated.Encrypt("new-secret")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(newStored, "v2:k2:") {
		t.Fatalf("rotated ciphertext = %q, want v2:k2 prefix", newStored)
	}
}

func TestKeyringRejectsUnknownKeyVersion(t *testing.T) {
	ring, _ := NewKeyring("k2", []Key{{ID: "k2", Key: testKey(2)}})
	old, _ := NewKeyring("k1", []Key{{ID: "k1", Key: testKey(1)}})
	stored, _ := old.Encrypt("secret")
	if _, err := ring.Decrypt(stored); !errors.Is(err, ErrDecrypt) {
		t.Fatalf("Decrypt() error = %v, want ErrDecrypt", err)
	}
}

func TestKeyringDecryptsLegacyV1Values(t *testing.T) {
	legacy, err := New(testKey(1))
	if err != nil {
		t.Fatal(err)
	}
	stored, err := legacy.Encrypt("legacy-secret")
	if err != nil {
		t.Fatal(err)
	}
	ring, err := NewKeyring("k2", []Key{
		{ID: "k2", Key: testKey(2)},
		{ID: "k1", Key: testKey(1)},
	})
	if err != nil {
		t.Fatal(err)
	}
	plain, err := ring.Decrypt(stored)
	if err != nil || plain != "legacy-secret" {
		t.Fatalf("legacy Decrypt() = %q, %v", plain, err)
	}
}

func TestKeyringRejectsTamperedCiphertext(t *testing.T) {
	ring, _ := NewKeyring("k1", []Key{{ID: "k1", Key: testKey(1)}})
	stored, _ := ring.EncryptWithAAD("secret", "aad")
	// Flip one base64 character in the payload.
	tampered := stored[:len(stored)-2] + "AA"
	if _, err := ring.DecryptWithAAD(tampered, "aad"); err == nil {
		t.Fatal("tampered ciphertext decrypted")
	}
}

func TestKeyringValidation(t *testing.T) {
	if _, err := NewKeyring("bad id", []Key{{ID: "k1", Key: testKey(1)}}); !errors.Is(err, ErrInvalidKeyring) {
		t.Fatalf("invalid active id error = %v", err)
	}
	if _, err := NewKeyring("k1", []Key{{ID: "k1", Key: make([]byte, 8)}}); !errors.Is(err, ErrInvalidKeyring) {
		t.Fatalf("short key error = %v", err)
	}
	if _, err := NewKeyring("k1", []Key{{ID: "k1", Key: testKey(1)}, {ID: "k1", Key: testKey(2)}}); !errors.Is(err, ErrInvalidKeyring) {
		t.Fatalf("duplicate id error = %v", err)
	}
	if _, err := NewKeyring("missing", []Key{{ID: "k1", Key: testKey(1)}}); !errors.Is(err, ErrInvalidKeyring) {
		t.Fatalf("missing active key error = %v", err)
	}
}

func TestKeyringFromHex(t *testing.T) {
	ring, err := NewKeyringFromHex("k1", strings.Repeat("ab", 32), map[string]string{"k0": strings.Repeat("cd", 32)})
	if err != nil {
		t.Fatal(err)
	}
	stored, err := ring.Encrypt("secret")
	if err != nil || !strings.HasPrefix(stored, "v2:k1:") {
		t.Fatalf("Encrypt() = %q, %v", stored, err)
	}
	if _, err := NewKeyringFromHex("k1", "not-hex", nil); !errors.Is(err, ErrInvalidKeyring) {
		t.Fatalf("invalid hex error = %v", err)
	}
	_ = ring.ActiveKeyID()
}
