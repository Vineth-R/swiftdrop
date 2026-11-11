package objectstore

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Local keeps objects beneath one private directory. It is the default
// development provider and supports streaming downloads but not presigned URLs.
type Local struct {
	root      *os.Root
	directory string
}

// NewLocal opens a private directory used only for object storage.
func NewLocal(directory string) (*Local, error) {
	directory = strings.TrimSpace(directory)
	if directory == "" {
		return nil, fmt.Errorf("%w: storage directory is empty", ErrInvalidConfig)
	}
	absolute, err := filepath.Abs(directory)
	if err != nil {
		return nil, fmt.Errorf("resolve object storage directory: %w", err)
	}
	if err := os.MkdirAll(absolute, 0o700); err != nil {
		return nil, fmt.Errorf("create object storage directory: %w", err)
	}
	// Windows uses ACLs rather than Unix mode bits. The directory remains
	// outside the public web tree, while deployment owns its ACL.
	if runtime.GOOS != "windows" {
		if err := os.Chmod(absolute, 0o700); err != nil {
			return nil, fmt.Errorf("secure object storage directory: %w", err)
		}
	}
	root, err := os.OpenRoot(absolute)
	if err != nil {
		return nil, fmt.Errorf("open object storage directory: %w", err)
	}
	return &Local{root: root, directory: absolute}, nil
}

func (s *Local) Put(ctx context.Context, key string, data []byte, _ string) error {
	if s == nil || s.root == nil {
		return ErrUnavailable
	}
	if !ValidKey(key) {
		return ErrUnsafeKey
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	directory := path.Dir(key)
	if directory != "." {
		if err := s.root.MkdirAll(filepath.FromSlash(directory), 0o700); err != nil {
			return fmt.Errorf("create object directory: %w", err)
		}
	}
	file, err := s.root.OpenFile(filepath.FromSlash(key), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("create object: %w", err)
	}
	if err := writeLocalFile(ctx, file, data); err != nil {
		_ = s.root.Remove(filepath.FromSlash(key))
		return err
	}
	return nil
}

func writeLocalFile(ctx context.Context, file *os.File, data []byte) error {
	if err := ctx.Err(); err != nil {
		_ = file.Close()
		return err
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return fmt.Errorf("write object: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return fmt.Errorf("sync object: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close object: %w", err)
	}
	return ctx.Err()
}

func (s *Local) Delete(_ context.Context, key string) error {
	if s == nil || s.root == nil {
		return ErrUnavailable
	}
	if !ValidKey(key) {
		return ErrUnsafeKey
	}
	if err := s.root.Remove(filepath.FromSlash(key)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("delete object: %w", err)
	}
	return nil
}

func (s *Local) Exists(_ context.Context, key string) (bool, error) {
	if s == nil || s.root == nil {
		return false, ErrUnavailable
	}
	if !ValidKey(key) {
		return false, ErrUnsafeKey
	}
	if _, err := s.root.Stat(filepath.FromSlash(key)); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("stat object: %w", err)
	}
	return true, nil
}

func (s *Local) PresignGet(context.Context, string, time.Duration) (string, time.Time, error) {
	return "", time.Time{}, ErrPresignUnsupported
}

func (s *Local) Open(_ context.Context, key string) (*Object, error) {
	if s == nil || s.root == nil {
		return nil, ErrUnavailable
	}
	if !ValidKey(key) {
		return nil, ErrUnsafeKey
	}
	file, err := s.root.Open(filepath.FromSlash(key))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("open object: %w", err)
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("stat object: %w", err)
	}
	return &Object{Body: file, Size: info.Size()}, nil
}

func (s *Local) Stat(_ context.Context, key string) (ObjectInfo, error) {
	if s == nil || s.root == nil {
		return ObjectInfo{}, ErrUnavailable
	}
	if !ValidKey(key) {
		return ObjectInfo{}, ErrUnsafeKey
	}
	info, err := s.root.Stat(filepath.FromSlash(key))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ObjectInfo{}, ErrNotFound
		}
		return ObjectInfo{}, fmt.Errorf("stat object: %w", err)
	}
	return ObjectInfo{Size: info.Size()}, nil
}

func (s *Local) List(ctx context.Context, prefix string) ([]string, error) {
	if s == nil || s.directory == "" {
		return nil, ErrUnavailable
	}
	prefix = strings.TrimPrefix(prefix, "/")
	keys := []string{}
	err := filepath.WalkDir(s.directory, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(s.directory, path)
		if err != nil {
			return err
		}
		key := filepath.ToSlash(relative)
		if prefix != "" && !strings.HasPrefix(key, prefix) {
			return nil
		}
		if ValidKey(key) {
			keys = append(keys, key)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list objects: %w", err)
	}
	return keys, nil
}

func (s *Local) Close() error {
	if s == nil || s.root == nil {
		return nil
	}
	return s.root.Close()
}
