// Package statemachine defines the canonical parcel lifecycle statuses and the
// deterministic rules for moving a parcel between them.
//
// The package is domain-only: it has no HTTP, database, or messaging
// dependencies. Parcel HTTP handlers, repositories, and the courier workflow
// should call these helpers before persisting a status change so the same rules
// are enforced everywhere.
package statemachine

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// ParcelStatus identifies a parcel's position in the delivery lifecycle.
type ParcelStatus string

const (
	StatusCreated        ParcelStatus = "CREATED"
	StatusPickedUp       ParcelStatus = "PICKED_UP"
	StatusInTransit      ParcelStatus = "IN_TRANSIT"
	StatusOutForDelivery ParcelStatus = "OUT_FOR_DELIVERY"
	StatusDelivered      ParcelStatus = "DELIVERED"
	StatusFailed         ParcelStatus = "FAILED"
)

// allStatuses is intentionally unexported so callers cannot mutate the
// canonical status set. Use AllStatuses for a defensive copy.
var allStatuses = [...]ParcelStatus{
	StatusCreated,
	StatusPickedUp,
	StatusInTransit,
	StatusOutForDelivery,
	StatusDelivered,
	StatusFailed,
}

// allowedTransitions is the single source of truth for permitted moves. It is
// unexported so callers cannot add or remove transitions at runtime. Use
// AllowedNext for a defensive copy.
var allowedTransitions = map[ParcelStatus][]ParcelStatus{
	StatusCreated:        {StatusPickedUp, StatusFailed},
	StatusPickedUp:       {StatusInTransit, StatusFailed},
	StatusInTransit:      {StatusOutForDelivery, StatusFailed},
	StatusOutForDelivery: {StatusDelivered, StatusFailed},
	StatusDelivered:      {},
	StatusFailed:         {},
}

// DomainError is a structured business error that maps to API v1.1 error codes.
type DomainError struct {
	HTTPStatus int    `json:"-"`
	Code       string `json:"code"`
	Message    string `json:"message"`
}

func (e *DomainError) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Is lets errors.Is match a dynamic domain error against a sentinel with the
// same API error code.
func (e *DomainError) Is(target error) bool {
	t, ok := target.(*DomainError)
	return ok && e != nil && t != nil && e.Code == t.Code
}

// Predefined domain errors per API v1.1.
var (
	ErrInvalidTransition = &DomainError{
		HTTPStatus: 409,
		Code:       "INVALID_TRANSITION",
		Message:    "status transition is not allowed",
	}
	ErrRecipientRequired = &DomainError{
		HTTPStatus: 422,
		Code:       "RECIPIENT_REQUIRED",
		Message:    "recipient name is mandatory when marking a parcel DELIVERED",
	}
	ErrFailureReasonRequired = &DomainError{
		HTTPStatus: 422,
		Code:       "FAILURE_REASON_REQUIRED",
		Message:    "a failure reason of 5 to 255 characters is mandatory when marking a parcel FAILED",
	}
	ErrValidationError = &DomainError{
		HTTPStatus: 422,
		Code:       "VALIDATION_ERROR",
		Message:    "validation failed",
	}
	ErrUnknownStatus = &DomainError{
		HTTPStatus: 422,
		Code:       "VALIDATION_ERROR",
		Message:    "unknown parcel status",
	}
	ErrParcelClosed = &DomainError{
		HTTPStatus: 409,
		Code:       "PARCEL_CLOSED",
		Message:    "closed parcels cannot be assigned or reassigned",
	}
	ErrForbidden = &DomainError{
		HTTPStatus: 403,
		Code:       "FORBIDDEN",
		Message:    "only the currently assigned courier can update parcel status",
	}
)

// NewValidationError creates a VALIDATION_ERROR with a custom detail message.
func NewValidationError(message string) *DomainError {
	return &DomainError{
		HTTPStatus: 422,
		Code:       "VALIDATION_ERROR",
		Message:    message,
	}
}

// NewUnknownStatusError creates a VALIDATION_ERROR describing an unknown status.
func NewUnknownStatusError(status ParcelStatus) *DomainError {
	return &DomainError{
		HTTPStatus: 422,
		Code:       "VALIDATION_ERROR",
		Message:    fmt.Sprintf("unknown parcel status %q", status),
	}
}

// NewInvalidTransitionError creates an INVALID_TRANSITION error with context.
func NewInvalidTransitionError(from, to ParcelStatus) *DomainError {
	return &DomainError{
		HTTPStatus: 409,
		Code:       "INVALID_TRANSITION",
		Message:    fmt.Sprintf("cannot transition parcel from %s to %s", from, to),
	}
}

// TransitionInput contains the metadata supplied with a status transition.
type TransitionInput struct {
	RecipientName string
	FailureReason string
	Note          string
}

// AllStatuses returns a defensive copy of the canonical status list.
func AllStatuses() []ParcelStatus {
	return append([]ParcelStatus(nil), allStatuses[:]...)
}

// AllowedNext returns the statuses reachable from from. The boolean is false
// when from is not a known status. The returned slice is a defensive copy.
func AllowedNext(from ParcelStatus) ([]ParcelStatus, bool) {
	next, ok := allowedTransitions[from]
	if !ok {
		return nil, false
	}
	return append([]ParcelStatus(nil), next...), true
}

// IsValidStatus reports whether status is one of the canonical statuses.
func IsValidStatus(status ParcelStatus) bool {
	for _, s := range allStatuses {
		if s == status {
			return true
		}
	}
	return false
}

// IsTerminal reports whether status is a terminal state (DELIVERED or FAILED).
func IsTerminal(status ParcelStatus) bool {
	return status == StatusDelivered || status == StatusFailed
}

// IsClosed reports whether a parcel in status is closed and therefore cannot be
// assigned or reassigned.
func IsClosed(status ParcelStatus) bool {
	return IsTerminal(status)
}

// CanTransition reports whether moving from one status to another is permitted.
// Unknown, identical, backward, and skipped transitions are rejected.
func CanTransition(from, to ParcelStatus) bool {
	nextStatuses, ok := allowedTransitions[from]
	if !ok {
		return false
	}
	for _, allowed := range nextStatuses {
		if allowed == to {
			return true
		}
	}
	return false
}

// CanAssignCourier checks whether a parcel in status is eligible for courier
// assignment. Terminal parcels are closed (409 PARCEL_CLOSED); unknown statuses
// are a validation error (422 VALIDATION_ERROR).
func CanAssignCourier(status ParcelStatus) error {
	if !IsValidStatus(status) {
		return NewUnknownStatusError(status)
	}
	if IsClosed(status) {
		return ErrParcelClosed
	}
	return nil
}

// VerifyCourierOwnership checks that only the courier currently assigned to the
// parcel can trigger a status update. Missing or mismatched identities return
// 403 FORBIDDEN.
func VerifyCourierOwnership(currentCourierID, actorCourierID string) error {
	if currentCourierID == "" || actorCourierID == "" || currentCourierID != actorCourierID {
		return ErrForbidden
	}
	return nil
}

// ValidateTransition validates a status transition and its required input:
//   - unknown or empty statuses return 422 VALIDATION_ERROR
//   - disallowed, identical, backward, skipped, or terminal transitions return
//     409 INVALID_TRANSITION
//   - DELIVERED requires a non-empty recipient name of at most 150 characters
//     (422 RECIPIENT_REQUIRED when missing)
//   - FAILED requires a failure reason of 5 to 255 characters
//     (422 FAILURE_REASON_REQUIRED when missing or out of range)
//   - the optional note is limited to 500 characters (422 VALIDATION_ERROR)
//
// Limits are measured in Unicode code points, matching PostgreSQL VARCHAR
// semantics.
func ValidateTransition(from, to ParcelStatus, input TransitionInput) error {
	if !IsValidStatus(from) {
		return NewUnknownStatusError(from)
	}
	if !IsValidStatus(to) {
		return NewUnknownStatusError(to)
	}
	if !CanTransition(from, to) {
		return NewInvalidTransitionError(from, to)
	}

	if to == StatusDelivered {
		recipient := strings.TrimSpace(input.RecipientName)
		if recipient == "" {
			return ErrRecipientRequired
		}
		if utf8.RuneCountInString(recipient) > 150 {
			return NewValidationError("recipient name must not exceed 150 characters")
		}
	}

	if to == StatusFailed {
		reason := strings.TrimSpace(input.FailureReason)
		reasonLength := utf8.RuneCountInString(reason)
		if reasonLength < 5 || reasonLength > 255 {
			return ErrFailureReasonRequired
		}
	}

	if utf8.RuneCountInString(input.Note) > 500 {
		return NewValidationError("note must not exceed 500 characters")
	}

	return nil
}

// ValidateCourierTransition combines courier ownership verification with status
// transition validation. Ownership is checked first so an unassigned actor
// cannot distinguish valid from invalid transitions.
func ValidateCourierTransition(currentCourierID, actorCourierID string, from, to ParcelStatus, input TransitionInput) error {
	if err := VerifyCourierOwnership(currentCourierID, actorCourierID); err != nil {
		return err
	}
	return ValidateTransition(from, to, input)
}
