// Package objectstore provides the shared private object-storage boundary used
// by SwiftDrop services. It has no business rules and never exposes provider
// credentials to callers.
package objectstore

import (
	"context"
	"errors"
	"io"
	"regexp"
	"strings"
	"time"
)

var (
	// ErrUnavailable means no storage provider is configured.
	ErrUnavailable = errors.New("object storage is not configured")
	// ErrInvalidConfig means the provider configuration is incomplete or unsafe.
	ErrInvalidConfig = errors.New("invalid object storage configuration")
	// ErrUnsafeKey means a caller supplied a key outside the generated format.
	ErrUnsafeKey = errors.New("object key is unsafe")
	// ErrNotFound means the requested object does not exist.
	ErrNotFound = errors.New("object not found")
	// ErrPresignUnsupported means the provider cannot issue signed URLs.
	ErrPresignUnsupported = errors.New("presigned URLs are not supported by this provider")
)

// Object is a private object stream returned by Open.
type Object struct {
	Body        io.ReadCloser
	Size        int64
	ContentType string
}

// ObjectInfo is metadata returned by Stat.
type ObjectInfo struct {
	Size        int64
	ContentType string
}

// Store persists opaque private objects under generated keys.
type Store interface {
	Put(ctx context.Context, key string, data []byte, contentType string) error
	Delete(ctx context.Context, key string) error
	Exists(ctx context.Context, key string) (bool, error)
	// PresignGet returns a short-lived download URL and its expiry.
	PresignGet(ctx context.Context, key string, ttl time.Duration) (string, time.Time, error)
	// Open streams an object for providers that cannot presign.
	Open(ctx context.Context, key string) (*Object, error)
	// Stat returns object metadata without downloading the body.
	Stat(ctx context.Context, key string) (ObjectInfo, error)
	// List returns keys under a prefix. It is used by reconciliation tooling.
	List(ctx context.Context, prefix string) ([]string, error)
	Close() error
}

var (
	uuidKeyPattern   = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}/[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\.(jpg|png)$`)
	legacyKeyPattern = regexp.MustCompile(`^[0-9a-f]{32}\.(jpg|png)$`)
)

// ValidKey accepts the generated <parcel-uuid>/<proof-uuid>.jpg|png key and
// the legacy flat <32 hex>.jpg|png key so existing proof rows keep working.
func ValidKey(key string) bool {
	if len(key) == 0 || len(key) > 255 {
		return false
	}
	if strings.Contains(key, `\`) || strings.Contains(key, "..") {
		return false
	}
	return uuidKeyPattern.MatchString(key) || legacyKeyPattern.MatchString(key)
}
