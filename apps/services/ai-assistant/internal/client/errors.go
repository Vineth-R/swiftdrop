package client

import (
	"errors"
	"fmt"
)

// Kind classifies an upstream failure so tools can map it to a safe tool
// status without inspecting response bodies.
type Kind int

const (
	// KindUnavailable covers timeouts, network failures, 429s and 5xx
	// responses.
	KindUnavailable Kind = iota
	// KindInvalid covers 400, 413 and 422 responses.
	KindInvalid
	// KindNotFound covers 404 responses.
	KindNotFound
	// KindUnauthorized covers 401 and 403 responses.
	KindUnauthorized
)

// Error is the typed failure returned by every client call. It deliberately
// carries only the HTTP status and the upstream error code; the response body,
// headers and any credentials are never retained.
type Error struct {
	Kind   Kind
	Status int
	Code   string
}

func (e *Error) Error() string {
	if e == nil {
		return "upstream request failed"
	}
	if e.Code != "" {
		return fmt.Sprintf("upstream request failed: status %d code %s", e.Status, e.Code)
	}
	return fmt.Sprintf("upstream request failed: status %d", e.Status)
}

// IsKind reports whether err is an *Error with the given kind.
func IsKind(err error, kind Kind) bool {
	var target *Error
	return errors.As(err, &target) && target.Kind == kind
}
