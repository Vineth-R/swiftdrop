package proof

import (
	"errors"
	"testing"
)

func TestNewKey(t *testing.T) {
	parcelID := "091d1c45-f200-4e53-8aef-afcfd3cb421d"
	proofID := "12e5e098-362a-474e-b86a-c4f081bea0db"
	for mimeType, extension := range map[string]string{"image/jpeg": ".jpg", "image/png": ".png"} {
		key, err := NewKey(parcelID, proofID, mimeType)
		if err != nil {
			t.Fatal(err)
		}
		if key != parcelID+"/"+proofID+extension {
			t.Fatalf("key = %q", key)
		}
		if len(key) > 255 {
			t.Fatalf("key exceeds photo_path limit: %d", len(key))
		}
	}
}

func TestNewKeyRejectsInvalidInput(t *testing.T) {
	if _, err := NewKey("not-a-uuid", "12e5e098-362a-474e-b86a-c4f081bea0db", "image/jpeg"); !errors.Is(err, ErrInvalidStorageKey) {
		t.Fatalf("invalid parcel error = %v", err)
	}
	if _, err := NewKey("091d1c45-f200-4e53-8aef-afcfd3cb421d", "not-a-uuid", "image/jpeg"); !errors.Is(err, ErrInvalidStorageKey) {
		t.Fatalf("invalid proof error = %v", err)
	}
	if _, err := NewKey("091d1c45-f200-4e53-8aef-afcfd3cb421d", "12e5e098-362a-474e-b86a-c4f081bea0db", "image/gif"); !errors.Is(err, ErrUnsupportedPhotoType) {
		t.Fatalf("invalid mime error = %v", err)
	}
}
