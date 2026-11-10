package proof

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

var (
	ErrStorageUnavailable = errors.New("proof storage is unavailable")
	ErrUnsafeStorageKey   = errors.New("proof storage key is unsafe")
)

// Storage persists validated proof bytes under an opaque private key.
type Storage interface {
	Save(context.Context, []byte, string) (string, error)
	Delete(context.Context, string) error
}

// LocalStorage keeps proof photos beneath one private directory. Callers only
// receive generated file names; the root path never enters API or database data.
type LocalStorage struct {
	root   *os.Root
	random io.Reader
}

// NewLocalStorage opens a private directory used only for delivery-proof files.
func NewLocalStorage(directory string) (*LocalStorage, error) {
	directory = strings.TrimSpace(directory)
	if directory == "" {
		return nil, fmt.Errorf("%w: storage directory is empty", ErrStorageUnavailable)
	}
	absolute, err := filepath.Abs(directory)
	if err != nil {
		return nil, fmt.Errorf("resolve proof storage directory: %w", err)
	}
	if err := os.MkdirAll(absolute, 0o700); err != nil {
		return nil, fmt.Errorf("create proof storage directory: %w", err)
	}
	// Windows uses ACLs rather than Unix mode bits. The directory remains outside
	// the public web tree, while deployment owns its ACL.
	if runtime.GOOS != "windows" {
		if err := os.Chmod(absolute, 0o700); err != nil {
			return nil, fmt.Errorf("secure proof storage directory: %w", err)
		}
	}
	root, err := os.OpenRoot(absolute)
	if err != nil {
		return nil, fmt.Errorf("open proof storage directory: %w", err)
	}
	return &LocalStorage{root: root, random: rand.Reader}, nil
}

// Save writes validated bytes under an unpredictable name with private mode.
func (s *LocalStorage) Save(ctx context.Context, data []byte, mimeType string) (string, error) {
	if s == nil || s.root == nil || s.random == nil {
		return "", ErrStorageUnavailable
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	extension := ".jpg"
	if mimeType == "image/png" {
		extension = ".png"
	} else if mimeType != "image/jpeg" {
		return "", fmt.Errorf("%w: unsupported verified MIME type", ErrUnsupportedPhotoType)
	}
	for range 4 {
		var token [16]byte
		if _, err := io.ReadFull(s.random, token[:]); err != nil {
			return "", fmt.Errorf("generate proof storage name: %w", err)
		}
		name := hex.EncodeToString(token[:]) + extension
		file, err := s.root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if err != nil {
			return "", fmt.Errorf("create proof photo: %w", err)
		}
		if err := writePrivateFile(ctx, file, data); err != nil {
			_ = file.Close()
			_ = s.root.Remove(name)
			return "", err
		}
		return name, nil
	}
	return "", errors.New("generate unique proof storage name")
}

func writePrivateFile(ctx context.Context, file *os.File, data []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, err := file.Write(data); err != nil {
		return fmt.Errorf("write proof photo: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync proof photo: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close proof photo: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}

// Delete removes a generated file. Arbitrary paths are rejected before the
// directory-scoped os.Root operation is attempted.
func (s *LocalStorage) Delete(_ context.Context, name string) error {
	if s == nil || s.root == nil {
		return ErrStorageUnavailable
	}
	if !safeStorageName(name) {
		return ErrUnsafeStorageKey
	}
	if err := s.root.Remove(name); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("delete proof photo: %w", err)
	}
	return nil
}

func safeStorageName(name string) bool {
	if len(name) != 36 || filepath.Base(name) != name || !filepath.IsLocal(name) {
		return false
	}
	extension := name[32:]
	if extension != ".jpg" && extension != ".png" {
		return false
	}
	_, err := hex.DecodeString(name[:32])
	return err == nil
}

func (s *LocalStorage) Close() error {
	if s == nil || s.root == nil {
		return nil
	}
	return s.root.Close()
}
