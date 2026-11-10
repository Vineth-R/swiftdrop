package proof

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLocalStorageSaveAndDelete(t *testing.T) {
	directory := t.TempDir()
	storage, err := NewLocalStorage(directory)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = storage.Close() })

	for _, tc := range []struct {
		name, mime, extension string
	}{
		{name: "JPEG", mime: "image/jpeg", extension: ".jpg"},
		{name: "PNG", mime: "image/png", extension: ".png"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := []byte("private proof bytes")
			key, err := storage.Save(t.Context(), data, tc.mime)
			if err != nil {
				t.Fatal(err)
			}
			if !safeStorageName(key) || !strings.HasSuffix(key, tc.extension) {
				t.Fatalf("unsafe storage key %q", key)
			}
			stored, err := os.ReadFile(filepath.Join(directory, key))
			if err != nil || !bytes.Equal(stored, data) {
				t.Fatalf("stored data = %q, error = %v", stored, err)
			}
			if err := storage.Delete(t.Context(), key); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(filepath.Join(directory, key)); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("deleted file stat error = %v", err)
			}
		})
	}
}

func TestLocalStorageRejectsUnsafeInputs(t *testing.T) {
	storage, err := NewLocalStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = storage.Close() })
	for _, name := range []string{"../photo.jpg", "photo.jpg", strings.Repeat("a", 32) + ".gif", `folder\\photo.jpg`} {
		if err := storage.Delete(t.Context(), name); !errors.Is(err, ErrUnsafeStorageKey) {
			t.Fatalf("Delete(%q) error = %v", name, err)
		}
	}
	if _, err := storage.Save(t.Context(), []byte("x"), "image/gif"); !errors.Is(err, ErrStorageUnavailable) {
		t.Fatalf("unsupported MIME error = %v", err)
	}
}

func TestLocalStorageFailureAndCancellation(t *testing.T) {
	if _, err := NewLocalStorage(" "); !errors.Is(err, ErrStorageUnavailable) {
		t.Fatalf("empty directory error = %v", err)
	}
	storage, err := NewLocalStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = storage.Close() })
	storage.random = bytes.NewReader(make([]byte, 8))
	if _, err := storage.Save(t.Context(), []byte("x"), "image/jpeg"); err == nil {
		t.Fatal("short random source succeeded")
	}
	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := storage.Save(cancelled, []byte("x"), "image/jpeg"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled save error = %v", err)
	}
}
