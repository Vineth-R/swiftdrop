package objectstore

import (
	"context"
	"errors"
	"io"
	"testing"
)

const (
	testParcelID = "091d1c45-f200-4e53-8aef-afcfd3cb421d"
	testProofID  = "12e5e098-362a-474e-b86a-c4f081bea0db"
)

func TestLocalPutOpenExistsDelete(t *testing.T) {
	local, err := NewLocal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = local.Close() })

	key := testParcelID + "/" + testProofID + ".jpg"
	if err := local.Put(t.Context(), key, []byte("private bytes"), "image/jpeg"); err != nil {
		t.Fatal(err)
	}
	exists, err := local.Exists(t.Context(), key)
	if err != nil || !exists {
		t.Fatalf("exists = %v, error = %v", exists, err)
	}
	object, err := local.Open(t.Context(), key)
	if err != nil {
		t.Fatal(err)
	}
	defer object.Body.Close()
	data, err := io.ReadAll(object.Body)
	if err != nil || string(data) != "private bytes" || object.Size != int64(len(data)) {
		t.Fatalf("data = %q, size = %d, error = %v", data, object.Size, err)
	}
	if err := local.Delete(t.Context(), key); err != nil {
		t.Fatal(err)
	}
	exists, err = local.Exists(t.Context(), key)
	if err != nil || exists {
		t.Fatalf("exists after delete = %v, error = %v", exists, err)
	}
	if object, err := local.Open(t.Context(), key); !errors.Is(err, ErrNotFound) {
		t.Fatalf("open missing = %+v, %v", object, err)
	}
}

func TestLocalSupportsLegacyFlatKeys(t *testing.T) {
	local, err := NewLocal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = local.Close() })

	key := "00000000000000000000000000000000.png"
	if err := local.Put(t.Context(), key, []byte("legacy"), "image/png"); err != nil {
		t.Fatal(err)
	}
	object, err := local.Open(t.Context(), key)
	if err != nil {
		t.Fatal(err)
	}
	_ = object.Body.Close()
}

func TestLocalRejectsUnsafeKeys(t *testing.T) {
	local, err := NewLocal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = local.Close() })

	ctx := context.Background()
	for _, key := range []string{"../escape.jpg", "photo.jpg", "a/../../x.jpg", testParcelID + "/x.gif"} {
		if err := local.Put(ctx, key, []byte("x"), "image/jpeg"); !errors.Is(err, ErrUnsafeKey) {
			t.Fatalf("Put(%q) = %v", key, err)
		}
		if err := local.Delete(ctx, key); !errors.Is(err, ErrUnsafeKey) {
			t.Fatalf("Delete(%q) = %v", key, err)
		}
		if _, err := local.Exists(ctx, key); !errors.Is(err, ErrUnsafeKey) {
			t.Fatalf("Exists(%q) = %v", key, err)
		}
	}
}

func TestLocalPresignUnsupported(t *testing.T) {
	local, err := NewLocal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = local.Close() })

	if _, _, err := local.PresignGet(t.Context(), testParcelID+"/"+testProofID+".jpg", DefaultPresignTTL); !errors.Is(err, ErrPresignUnsupported) {
		t.Fatalf("PresignGet() = %v", err)
	}
}
