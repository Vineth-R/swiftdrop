package statemachine

import (
	"fmt"
	"strings"
)

// ParcelStatus represents the status of a parcel in the lifecycle.
type ParcelStatus string

const (
	StatusCreated        ParcelStatus = "CREATED"
	StatusPickedUp       ParcelStatus = "PICKED_UP"
	StatusInTransit      ParcelStatus = "IN_TRANSIT"
	StatusOutForDelivery ParcelStatus = "OUT_FOR_DELIVERY"
	StatusDelivered      ParcelStatus = "DELIVERED"
	StatusFailed         ParcelStatus = "FAILED"
)

// AllStatuses is the list of all valid parcel statuses.
var AllStatuses = []ParcelStatus{
	StatusCreated,
	StatusPickedUp,
	StatusInTransit,
	StatusOutForDelivery,
	StatusDelivered,
	StatusFailed,
}

// AllowedTransitions maps each status to its allowed next statuses.
var AllowedTransitions = map[ParcelStatus][]ParcelStatus{
	StatusCreated:        {StatusPickedUp, StatusFailed},
	StatusPickedUp:       {StatusInTransit, StatusFailed},
	StatusInTransit:      {StatusOutForDelivery, StatusFailed},
	StatusOutForDelivery: {StatusDelivered, StatusFailed},
	StatusDelivered:      {},
	StatusFailed:         {},
}

// DomainError represents a structured business domain error mapping to API v1.1.
type DomainError struct {
	HTTPStatus int    `json:"-"`
	Code       string `json:"code"`
	Message    string `json:"message"`
}

func (e *DomainError) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Predefined domain errors per API v1.1 specification.
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

// NewValidationError creates a VALIDATION_ERROR with custom detail message.
func NewValidationError(message string) *DomainError {
	return &DomainError{
		HTTPStatus: 422,
		Code:       "VALIDATION_ERROR",
		Message:    message,
	}
}

// NewInvalidTransitionError creates an INVALID_TRANSITION error with transition context.
func NewInvalidTransitionError(from, to ParcelStatus) *DomainError {
	return &DomainError{
		HTTPStatus: 409,
		Code:       "INVALID_TRANSITION",
		Message:    fmt.Sprintf("cannot transition parcel from %s to %s", from, to),
	}
}

// TransitionInput contains metadata provided when transitioning parcel status.
type TransitionInput struct {
	RecipientName string
	FailureReason string
	Note          string
}

// IsValidStatus checks if a given status string is a known ParcelStatus.
func IsValidStatus(status ParcelStatus) bool {
	for _, s := range AllStatuses {
		if s == status {
			return true
		}
	}
	return false
}

// IsTerminal returns true if the status is a terminal state (DELIVERED or FAILED).
func IsTerminal(status ParcelStatus) bool {
	return status == StatusDelivered || status == StatusFailed
}

// IsClosed returns true if the parcel is closed and cannot be assigned/reassigned.
func IsClosed(status ParcelStatus) bool {
	return IsTerminal(status)
}

// CanTransition checks if transitioning from one status to another is permitted.
func CanTransition(from, to ParcelStatus) bool {
	nextStatuses, ok := AllowedTransitions[from]
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

// CanAssignCourier checks if a parcel in the given status is eligible for courier assignment.
// A parcel in DELIVERED or FAILED status cannot be assigned or reassigned -> 409 PARCEL_CLOSED.
func CanAssignCourier(status ParcelStatus) error {
	if IsClosed(status) {
		return ErrParcelClosed
	}
	return nil
}

// VerifyCourierOwnership checks that only the courier currently assigned to the parcel
// can trigger a status update. If not matching, returns 403 FORBIDDEN.
func VerifyCourierOwnership(currentCourierID, actorCourierID string) error {
	if currentCourierID == "" || actorCourierID == "" || currentCourierID != actorCourierID {
		return ErrForbidden
	}
	return nil
}

// ValidateTransition validates that the transition from 'from' to 'to' is allowed
// and verifies that all transition input rules are satisfied according to API v1.1:
// - Disallowed status transition -> 409 INVALID_TRANSITION
// - Transitioning to DELIVERED requires a non-empty RecipientName (up to 150 chars) -> missing returns 422 RECIPIENT_REQUIRED
// - Transitioning to FAILED requires a FailureReason between 5 and 255 chars -> missing/invalid returns 422 FAILURE_REASON_REQUIRED
// - Optional Note is allowed up to 500 characters -> exceeding returns 422 VALIDATION_ERROR
func ValidateTransition(from, to ParcelStatus, input TransitionInput) error {
	if !CanTransition(from, to) {
		return NewInvalidTransitionError(from, to)
	}

	if to == StatusDelivered {
		recipient := strings.TrimSpace(input.RecipientName)
		if recipient == "" {
			return ErrRecipientRequired
		}
		if len(recipient) > 150 {
			return NewValidationError("recipient name must not exceed 150 characters")
		}
	}

	if to == StatusFailed {
		reason := strings.TrimSpace(input.FailureReason)
		if len(reason) < 5 || len(reason) > 255 {
			return ErrFailureReasonRequired
		}
	}

	if len(input.Note) > 500 {
		return NewValidationError("note must not exceed 500 characters")
	}

	return nil
}

// ValidateCourierTransition combines ownership verification with status transition validation.
func ValidateCourierTransition(currentCourierID, actorCourierID string, from, to ParcelStatus, input TransitionInput) error {
	if err := VerifyCourierOwnership(currentCourierID, actorCourierID); err != nil {
		return err
	}
	return ValidateTransition(from, to, input)
}

