package statemachine_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/Vineth-R/swiftdrop/apps/services/parcel/internal/statemachine"
)

// expectedTransitions is an independent copy of the API v1.1 transition table.
// Keeping it separate from the package's internal map makes a wrongly-permitted
// transition fail instead of silently agreeing with the implementation.
var expectedTransitions = map[statemachine.ParcelStatus][]statemachine.ParcelStatus{
	statemachine.StatusCreated:        {statemachine.StatusPickedUp, statemachine.StatusFailed},
	statemachine.StatusPickedUp:       {statemachine.StatusInTransit, statemachine.StatusFailed},
	statemachine.StatusInTransit:      {statemachine.StatusOutForDelivery, statemachine.StatusFailed},
	statemachine.StatusOutForDelivery: {statemachine.StatusDelivered, statemachine.StatusFailed},
	statemachine.StatusDelivered:      {},
	statemachine.StatusFailed:         {},
}

func expectedStatuses() []statemachine.ParcelStatus {
	return []statemachine.ParcelStatus{
		statemachine.StatusCreated,
		statemachine.StatusPickedUp,
		statemachine.StatusInTransit,
		statemachine.StatusOutForDelivery,
		statemachine.StatusDelivered,
		statemachine.StatusFailed,
	}
}

func containsStatus(statuses []statemachine.ParcelStatus, target statemachine.ParcelStatus) bool {
	for _, status := range statuses {
		if status == target {
			return true
		}
	}
	return false
}

func TestCanTransitionMatrix(t *testing.T) {
	statuses := expectedStatuses()
	allowedCount := 0
	prohibitedCount := 0

	for _, from := range statuses {
		for _, to := range statuses {
			want := containsStatus(expectedTransitions[from], to)
			got := statemachine.CanTransition(from, to)
			if got != want {
				t.Errorf("CanTransition(%s, %s) = %v; want %v", from, to, got, want)
			}
			if want {
				allowedCount++
			} else {
				prohibitedCount++
			}
		}
	}

	if allowedCount != 8 {
		t.Errorf("allowed transition count = %d; want 8", allowedCount)
	}
	if prohibitedCount != len(statuses)*len(statuses)-allowedCount {
		t.Errorf("prohibited transition count = %d; want %d", prohibitedCount, len(statuses)*len(statuses)-allowedCount)
	}
}

func TestCanTransitionRejectsUnknownAndEmptyStatuses(t *testing.T) {
	tests := []struct {
		name string
		from statemachine.ParcelStatus
		to   statemachine.ParcelStatus
	}{
		{name: "unknown source", from: statemachine.ParcelStatus("UNKNOWN"), to: statemachine.StatusPickedUp},
		{name: "unknown destination", from: statemachine.StatusCreated, to: statemachine.ParcelStatus("UNKNOWN")},
		{name: "empty source", from: statemachine.ParcelStatus(""), to: statemachine.StatusPickedUp},
		{name: "empty destination", from: statemachine.StatusCreated, to: statemachine.ParcelStatus("")},
		{name: "both empty", from: statemachine.ParcelStatus(""), to: statemachine.ParcelStatus("")},
		{name: "lowercase source", from: statemachine.ParcelStatus("created"), to: statemachine.StatusPickedUp},
		{name: "whitespace destination", from: statemachine.StatusCreated, to: statemachine.ParcelStatus(" PICKED_UP ")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if statemachine.CanTransition(tt.from, tt.to) {
				t.Fatalf("CanTransition(%q, %q) = true; want false", tt.from, tt.to)
			}
		})
	}
}

func TestAllowedNextReturnsDefensiveCopies(t *testing.T) {
	for _, status := range expectedStatuses() {
		got, ok := statemachine.AllowedNext(status)
		if !ok {
			t.Fatalf("AllowedNext(%s) returned ok=false; want true", status)
		}
		want := expectedTransitions[status]
		if len(got) != len(want) {
			t.Fatalf("AllowedNext(%s) length = %d; want %d", status, len(got), len(want))
		}
		for _, expected := range want {
			if !containsStatus(got, expected) {
				t.Errorf("AllowedNext(%s) missing %s; got %v", status, expected, got)
			}
		}
	}

	if next, ok := statemachine.AllowedNext(statemachine.ParcelStatus("UNKNOWN")); ok || next != nil {
		t.Errorf("AllowedNext(UNKNOWN) = (%v, %v); want (nil, false)", next, ok)
	}
	if next, ok := statemachine.AllowedNext(statemachine.ParcelStatus("")); ok || next != nil {
		t.Errorf("AllowedNext(empty) = (%v, %v); want (nil, false)", next, ok)
	}

	// Mutating the returned slice must not change the canonical table.
	next, _ := statemachine.AllowedNext(statemachine.StatusCreated)
	next[0] = statemachine.StatusDelivered
	again, _ := statemachine.AllowedNext(statemachine.StatusCreated)
	if again[0] != statemachine.StatusPickedUp {
		t.Fatalf("AllowedNext returned a shared slice: first element = %s; want PICKED_UP", again[0])
	}

	statuses := statemachine.AllStatuses()
	statuses[0] = statemachine.ParcelStatus("MUTATED")
	if statemachine.AllStatuses()[0] != statemachine.StatusCreated {
		t.Fatal("AllStatuses returned a shared slice")
	}
}

func TestAllStatusesAreCanonical(t *testing.T) {
	statuses := statemachine.AllStatuses()
	if len(statuses) != 6 {
		t.Fatalf("AllStatuses length = %d; want 6", len(statuses))
	}
	seen := make(map[statemachine.ParcelStatus]bool, len(statuses))
	for _, status := range statuses {
		if seen[status] {
			t.Fatalf("AllStatuses contains duplicate %s", status)
		}
		seen[status] = true
		if !statemachine.IsValidStatus(status) {
			t.Fatalf("IsValidStatus(%s) = false; want true", status)
		}
		if _, ok := statemachine.AllowedNext(status); !ok {
			t.Fatalf("AllowedNext(%s) missing from transition table", status)
		}
	}
}

func TestValidateTransitionRejectsUnknownAndEmptyStatuses(t *testing.T) {
	tests := []struct {
		name string
		from statemachine.ParcelStatus
		to   statemachine.ParcelStatus
	}{
		{name: "unknown source", from: statemachine.ParcelStatus("UNKNOWN"), to: statemachine.StatusPickedUp},
		{name: "unknown destination", from: statemachine.StatusCreated, to: statemachine.ParcelStatus("UNKNOWN")},
		{name: "empty source", from: statemachine.ParcelStatus(""), to: statemachine.StatusPickedUp},
		{name: "empty destination", from: statemachine.StatusCreated, to: statemachine.ParcelStatus("")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := statemachine.ValidateTransition(tt.from, tt.to, statemachine.TransitionInput{})
			assertDomainError(t, err, "VALIDATION_ERROR", 422)
			if !errors.Is(err, statemachine.ErrUnknownStatus) {
				t.Errorf("errors.Is(err, ErrUnknownStatus) = false; err = %v", err)
			}
		})
	}
}

func TestValidateTransitionRejectsProhibitedTransitions(t *testing.T) {
	tests := []struct {
		name string
		from statemachine.ParcelStatus
		to   statemachine.ParcelStatus
	}{
		{name: "skip CREATED to IN_TRANSIT", from: statemachine.StatusCreated, to: statemachine.StatusInTransit},
		{name: "skip CREATED to DELIVERED", from: statemachine.StatusCreated, to: statemachine.StatusDelivered},
		{name: "skip PICKED_UP to DELIVERED", from: statemachine.StatusPickedUp, to: statemachine.StatusDelivered},
		{name: "skip IN_TRANSIT to DELIVERED", from: statemachine.StatusInTransit, to: statemachine.StatusDelivered},
		{name: "backward PICKED_UP to CREATED", from: statemachine.StatusPickedUp, to: statemachine.StatusCreated},
		{name: "backward IN_TRANSIT to PICKED_UP", from: statemachine.StatusInTransit, to: statemachine.StatusPickedUp},
		{name: "backward OUT_FOR_DELIVERY to IN_TRANSIT", from: statemachine.StatusOutForDelivery, to: statemachine.StatusInTransit},
		{name: "self CREATED", from: statemachine.StatusCreated, to: statemachine.StatusCreated},
		{name: "self IN_TRANSIT", from: statemachine.StatusInTransit, to: statemachine.StatusInTransit},
		{name: "self DELIVERED", from: statemachine.StatusDelivered, to: statemachine.StatusDelivered},
		{name: "terminal DELIVERED to FAILED", from: statemachine.StatusDelivered, to: statemachine.StatusFailed},
		{name: "terminal FAILED to CREATED", from: statemachine.StatusFailed, to: statemachine.StatusCreated},
		{name: "terminal FAILED to DELIVERED", from: statemachine.StatusFailed, to: statemachine.StatusDelivered},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := statemachine.ValidateTransition(tt.from, tt.to, statemachine.TransitionInput{
				RecipientName: "Amali Fernando",
				FailureReason: "Recipient unavailable",
			})
			assertDomainError(t, err, "INVALID_TRANSITION", 409)
			if !errors.Is(err, statemachine.ErrInvalidTransition) {
				t.Errorf("errors.Is(err, ErrInvalidTransition) = false; err = %v", err)
			}
		})
	}
}

func TestValidateTransitionAllowsEveryPermittedTransition(t *testing.T) {
	for from, nextStatuses := range expectedTransitions {
		for _, to := range nextStatuses {
			t.Run(string(from)+" to "+string(to), func(t *testing.T) {
				err := statemachine.ValidateTransition(from, to, validInputFor(to))
				if err != nil {
					t.Fatalf("ValidateTransition(%s, %s) = %v; want nil", from, to, err)
				}
			})
		}
	}
}

func TestValidateTransitionInputRules(t *testing.T) {
	tests := []struct {
		name        string
		from        statemachine.ParcelStatus
		to          statemachine.ParcelStatus
		input       statemachine.TransitionInput
		wantCode    string
		wantHTTP    int
		wantIs      error
		wantNoError bool
	}{
		{
			name:     "DELIVERED missing recipient",
			from:     statemachine.StatusOutForDelivery,
			to:       statemachine.StatusDelivered,
			input:    statemachine.TransitionInput{},
			wantCode: "RECIPIENT_REQUIRED", wantHTTP: 422, wantIs: statemachine.ErrRecipientRequired,
		},
		{
			name:     "DELIVERED whitespace recipient",
			from:     statemachine.StatusOutForDelivery,
			to:       statemachine.StatusDelivered,
			input:    statemachine.TransitionInput{RecipientName: " \t\n "},
			wantCode: "RECIPIENT_REQUIRED", wantHTTP: 422, wantIs: statemachine.ErrRecipientRequired,
		},
		{
			name:        "DELIVERED recipient at 150 characters",
			from:        statemachine.StatusOutForDelivery,
			to:          statemachine.StatusDelivered,
			input:       statemachine.TransitionInput{RecipientName: strings.Repeat("A", 150)},
			wantNoError: true,
		},
		{
			name:     "DELIVERED recipient at 151 characters",
			from:     statemachine.StatusOutForDelivery,
			to:       statemachine.StatusDelivered,
			input:    statemachine.TransitionInput{RecipientName: strings.Repeat("A", 151)},
			wantCode: "VALIDATION_ERROR", wantHTTP: 422,
		},
		{
			name:        "DELIVERED multibyte recipient at 150 runes",
			from:        statemachine.StatusOutForDelivery,
			to:          statemachine.StatusDelivered,
			input:       statemachine.TransitionInput{RecipientName: strings.Repeat("é", 150)},
			wantNoError: true,
		},
		{
			name:     "DELIVERED multibyte recipient at 151 runes",
			from:     statemachine.StatusOutForDelivery,
			to:       statemachine.StatusDelivered,
			input:    statemachine.TransitionInput{RecipientName: strings.Repeat("é", 151)},
			wantCode: "VALIDATION_ERROR", wantHTTP: 422,
		},
		{
			name:     "FAILED reason at 4 characters",
			from:     statemachine.StatusOutForDelivery,
			to:       statemachine.StatusFailed,
			input:    statemachine.TransitionInput{FailureReason: "Nope"},
			wantCode: "FAILURE_REASON_REQUIRED", wantHTTP: 422, wantIs: statemachine.ErrFailureReasonRequired,
		},
		{
			name:        "FAILED reason at 5 characters",
			from:        statemachine.StatusOutForDelivery,
			to:          statemachine.StatusFailed,
			input:       statemachine.TransitionInput{FailureReason: "12345"},
			wantNoError: true,
		},
		{
			name:        "FAILED reason at 255 characters",
			from:        statemachine.StatusOutForDelivery,
			to:          statemachine.StatusFailed,
			input:       statemachine.TransitionInput{FailureReason: strings.Repeat("R", 255)},
			wantNoError: true,
		},
		{
			name:     "FAILED reason at 256 characters",
			from:     statemachine.StatusOutForDelivery,
			to:       statemachine.StatusFailed,
			input:    statemachine.TransitionInput{FailureReason: strings.Repeat("R", 256)},
			wantCode: "FAILURE_REASON_REQUIRED", wantHTTP: 422, wantIs: statemachine.ErrFailureReasonRequired,
		},
		{
			name:     "FAILED whitespace reason",
			from:     statemachine.StatusOutForDelivery,
			to:       statemachine.StatusFailed,
			input:    statemachine.TransitionInput{FailureReason: "     "},
			wantCode: "FAILURE_REASON_REQUIRED", wantHTTP: 422, wantIs: statemachine.ErrFailureReasonRequired,
		},
		{
			name:        "FAILED multibyte reason at 5 runes",
			from:        statemachine.StatusOutForDelivery,
			to:          statemachine.StatusFailed,
			input:       statemachine.TransitionInput{FailureReason: strings.Repeat("é", 5)},
			wantNoError: true,
		},
		{
			name:        "note at 500 characters",
			from:        statemachine.StatusCreated,
			to:          statemachine.StatusPickedUp,
			input:       statemachine.TransitionInput{Note: strings.Repeat("N", 500)},
			wantNoError: true,
		},
		{
			name:     "note at 501 characters",
			from:     statemachine.StatusCreated,
			to:       statemachine.StatusPickedUp,
			input:    statemachine.TransitionInput{Note: strings.Repeat("N", 501)},
			wantCode: "VALIDATION_ERROR", wantHTTP: 422, wantIs: statemachine.ErrValidationError,
		},
		{
			name:        "note multibyte at 500 runes",
			from:        statemachine.StatusCreated,
			to:          statemachine.StatusPickedUp,
			input:       statemachine.TransitionInput{Note: strings.Repeat("é", 500)},
			wantNoError: true,
		},
		{
			name:     "note multibyte at 501 runes",
			from:     statemachine.StatusCreated,
			to:       statemachine.StatusPickedUp,
			input:    statemachine.TransitionInput{Note: strings.Repeat("é", 501)},
			wantCode: "VALIDATION_ERROR", wantHTTP: 422, wantIs: statemachine.ErrValidationError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := statemachine.ValidateTransition(tt.from, tt.to, tt.input)
			if tt.wantNoError {
				if err != nil {
					t.Fatalf("ValidateTransition() = %v; want nil", err)
				}
				return
			}
			assertDomainError(t, err, tt.wantCode, tt.wantHTTP)
			if tt.wantIs != nil && !errors.Is(err, tt.wantIs) {
				t.Errorf("errors.Is(err, %v) = false; err = %v", tt.wantIs, err)
			}
		})
	}
}

func TestCanAssignCourier(t *testing.T) {
	tests := []struct {
		name     string
		status   statemachine.ParcelStatus
		wantCode string
		wantHTTP int
		wantIs   error
	}{
		{name: "CREATED", status: statemachine.StatusCreated},
		{name: "PICKED_UP", status: statemachine.StatusPickedUp},
		{name: "IN_TRANSIT", status: statemachine.StatusInTransit},
		{name: "OUT_FOR_DELIVERY", status: statemachine.StatusOutForDelivery},
		{name: "DELIVERED", status: statemachine.StatusDelivered, wantCode: "PARCEL_CLOSED", wantHTTP: 409, wantIs: statemachine.ErrParcelClosed},
		{name: "FAILED", status: statemachine.StatusFailed, wantCode: "PARCEL_CLOSED", wantHTTP: 409, wantIs: statemachine.ErrParcelClosed},
		{name: "unknown", status: statemachine.ParcelStatus("UNKNOWN"), wantCode: "VALIDATION_ERROR", wantHTTP: 422, wantIs: statemachine.ErrUnknownStatus},
		{name: "empty", status: statemachine.ParcelStatus(""), wantCode: "VALIDATION_ERROR", wantHTTP: 422, wantIs: statemachine.ErrUnknownStatus},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := statemachine.CanAssignCourier(tt.status)
			if tt.wantCode == "" {
				if err != nil {
					t.Fatalf("CanAssignCourier(%s) = %v; want nil", tt.status, err)
				}
				return
			}
			assertDomainError(t, err, tt.wantCode, tt.wantHTTP)
			if !errors.Is(err, tt.wantIs) {
				t.Errorf("errors.Is(err, %v) = false; err = %v", tt.wantIs, err)
			}
		})
	}
}

func TestVerifyCourierOwnership(t *testing.T) {
	tests := []struct {
		name             string
		currentCourierID string
		actorCourierID   string
		wantErr          bool
	}{
		{name: "matching identities", currentCourierID: "courier-1", actorCourierID: "courier-1"},
		{name: "different identities", currentCourierID: "courier-1", actorCourierID: "courier-2", wantErr: true},
		{name: "empty current identity", currentCourierID: "", actorCourierID: "courier-1", wantErr: true},
		{name: "empty actor identity", currentCourierID: "courier-1", actorCourierID: "", wantErr: true},
		{name: "both empty", currentCourierID: "", actorCourierID: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := statemachine.VerifyCourierOwnership(tt.currentCourierID, tt.actorCourierID)
			if !tt.wantErr {
				if err != nil {
					t.Fatalf("VerifyCourierOwnership() = %v; want nil", err)
				}
				return
			}
			assertDomainError(t, err, "FORBIDDEN", 403)
			if !errors.Is(err, statemachine.ErrForbidden) {
				t.Errorf("errors.Is(err, ErrForbidden) = false; err = %v", err)
			}
		})
	}
}

func TestValidateCourierTransition(t *testing.T) {
	tests := []struct {
		name             string
		currentCourierID string
		actorCourierID   string
		from             statemachine.ParcelStatus
		to               statemachine.ParcelStatus
		input            statemachine.TransitionInput
		wantCode         string
		wantHTTP         int
	}{
		{
			name:             "assigned courier valid transition",
			currentCourierID: "courier-1", actorCourierID: "courier-1",
			from: statemachine.StatusPickedUp, to: statemachine.StatusInTransit,
		},
		{
			name:             "ownership failure takes precedence over invalid transition",
			currentCourierID: "courier-1", actorCourierID: "courier-2",
			from: statemachine.StatusCreated, to: statemachine.StatusDelivered,
			wantCode: "FORBIDDEN", wantHTTP: 403,
		},
		{
			name:             "assigned courier invalid transition",
			currentCourierID: "courier-1", actorCourierID: "courier-1",
			from: statemachine.StatusCreated, to: statemachine.StatusDelivered,
			wantCode: "INVALID_TRANSITION", wantHTTP: 409,
		},
		{
			name:             "assigned courier missing recipient",
			currentCourierID: "courier-1", actorCourierID: "courier-1",
			from: statemachine.StatusOutForDelivery, to: statemachine.StatusDelivered,
			wantCode: "RECIPIENT_REQUIRED", wantHTTP: 422,
		},
		{
			name:             "assigned courier missing failure reason",
			currentCourierID: "courier-1", actorCourierID: "courier-1",
			from: statemachine.StatusOutForDelivery, to: statemachine.StatusFailed,
			wantCode: "FAILURE_REASON_REQUIRED", wantHTTP: 422,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := statemachine.ValidateCourierTransition(
				tt.currentCourierID, tt.actorCourierID, tt.from, tt.to, tt.input,
			)
			if tt.wantCode == "" {
				if err != nil {
					t.Fatalf("ValidateCourierTransition() = %v; want nil", err)
				}
				return
			}
			assertDomainError(t, err, tt.wantCode, tt.wantHTTP)
		})
	}
}

func TestStatusHelpers(t *testing.T) {
	if !statemachine.IsValidStatus(statemachine.StatusCreated) {
		t.Error("IsValidStatus(CREATED) = false; want true")
	}
	if statemachine.IsValidStatus("INVALID") {
		t.Error("IsValidStatus(INVALID) = true; want false")
	}
	if statemachine.IsValidStatus("") {
		t.Error("IsValidStatus(empty) = true; want false")
	}

	for _, status := range []statemachine.ParcelStatus{statemachine.StatusDelivered, statemachine.StatusFailed} {
		if !statemachine.IsTerminal(status) {
			t.Errorf("IsTerminal(%s) = false; want true", status)
		}
		if !statemachine.IsClosed(status) {
			t.Errorf("IsClosed(%s) = false; want true", status)
		}
	}
	for _, status := range []statemachine.ParcelStatus{
		statemachine.StatusCreated,
		statemachine.StatusPickedUp,
		statemachine.StatusInTransit,
		statemachine.StatusOutForDelivery,
		statemachine.ParcelStatus("UNKNOWN"),
		statemachine.ParcelStatus(""),
	} {
		if statemachine.IsTerminal(status) {
			t.Errorf("IsTerminal(%s) = true; want false", status)
		}
		if statemachine.IsClosed(status) {
			t.Errorf("IsClosed(%s) = true; want false", status)
		}
	}
}

func TestDomainErrorIsMatchesByCode(t *testing.T) {
	dynamic := statemachine.NewInvalidTransitionError(statemachine.StatusCreated, statemachine.StatusDelivered)
	if !errors.Is(dynamic, statemachine.ErrInvalidTransition) {
		t.Errorf("errors.Is(dynamic INVALID_TRANSITION, ErrInvalidTransition) = false")
	}
	if errors.Is(dynamic, statemachine.ErrForbidden) {
		t.Error("errors.Is(INVALID_TRANSITION, ErrForbidden) = true; want false")
	}

	unknown := statemachine.NewUnknownStatusError("BROKEN")
	if !errors.Is(unknown, statemachine.ErrUnknownStatus) {
		t.Errorf("errors.Is(unknown status, ErrUnknownStatus) = false")
	}
	if !errors.Is(unknown, statemachine.ErrValidationError) {
		t.Errorf("errors.Is(unknown status, ErrValidationError) = false")
	}
}

func validInputFor(status statemachine.ParcelStatus) statemachine.TransitionInput {
	input := statemachine.TransitionInput{Note: "handled by courier"}
	switch status {
	case statemachine.StatusDelivered:
		input.RecipientName = "Amali Fernando"
	case statemachine.StatusFailed:
		input.FailureReason = "Recipient unavailable after two attempts"
	}
	return input
}

func assertDomainError(t *testing.T, err error, wantCode string, wantHTTP int) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected %s error, got nil", wantCode)
	}
	var domainErr *statemachine.DomainError
	if !errors.As(err, &domainErr) {
		t.Fatalf("expected *statemachine.DomainError, got %T (%v)", err, err)
	}
	if domainErr.Code != wantCode {
		t.Errorf("error code = %s; want %s", domainErr.Code, wantCode)
	}
	if domainErr.HTTPStatus != wantHTTP {
		t.Errorf("HTTP status = %d; want %d", domainErr.HTTPStatus, wantHTTP)
	}
}
