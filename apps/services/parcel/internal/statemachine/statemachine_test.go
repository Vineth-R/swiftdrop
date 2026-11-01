package statemachine_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/Vineth-R/swiftdrop/apps/services/parcel/internal/statemachine"
)

func TestAllowedTransitions(t *testing.T) {
	tests := []struct {
		name     string
		from     statemachine.ParcelStatus
		to       statemachine.ParcelStatus
		expected bool
	}{
		// Allowed transitions
		{"CREATED to PICKED_UP", statemachine.StatusCreated, statemachine.StatusPickedUp, true},
		{"CREATED to FAILED", statemachine.StatusCreated, statemachine.StatusFailed, true},
		{"PICKED_UP to IN_TRANSIT", statemachine.StatusPickedUp, statemachine.StatusInTransit, true},
		{"PICKED_UP to FAILED", statemachine.StatusPickedUp, statemachine.StatusFailed, true},
		{"IN_TRANSIT to OUT_FOR_DELIVERY", statemachine.StatusInTransit, statemachine.StatusOutForDelivery, true},
		{"IN_TRANSIT to FAILED", statemachine.StatusInTransit, statemachine.StatusFailed, true},
		{"OUT_FOR_DELIVERY to DELIVERED", statemachine.StatusOutForDelivery, statemachine.StatusDelivered, true},
		{"OUT_FOR_DELIVERY to FAILED", statemachine.StatusOutForDelivery, statemachine.StatusFailed, true},

		// Refused: skipping steps
		{"CREATED to IN_TRANSIT (skip)", statemachine.StatusCreated, statemachine.StatusInTransit, false},
		{"CREATED to OUT_FOR_DELIVERY (skip)", statemachine.StatusCreated, statemachine.StatusOutForDelivery, false},
		{"CREATED to DELIVERED (skip)", statemachine.StatusCreated, statemachine.StatusDelivered, false},
		{"PICKED_UP to DELIVERED (skip)", statemachine.StatusPickedUp, statemachine.StatusDelivered, false},
		{"IN_TRANSIT to DELIVERED (skip)", statemachine.StatusInTransit, statemachine.StatusDelivered, false},

		// Refused: backward steps
		{"PICKED_UP to CREATED (backward)", statemachine.StatusPickedUp, statemachine.StatusCreated, false},
		{"IN_TRANSIT to PICKED_UP (backward)", statemachine.StatusInTransit, statemachine.StatusPickedUp, false},
		{"OUT_FOR_DELIVERY to IN_TRANSIT (backward)", statemachine.StatusOutForDelivery, statemachine.StatusInTransit, false},
		{"OUT_FOR_DELIVERY to PICKED_UP (backward)", statemachine.StatusOutForDelivery, statemachine.StatusPickedUp, false},

		// Refused: self transitions
		{"CREATED to CREATED (self)", statemachine.StatusCreated, statemachine.StatusCreated, false},
		{"PICKED_UP to PICKED_UP (self)", statemachine.StatusPickedUp, statemachine.StatusPickedUp, false},
		{"IN_TRANSIT to IN_TRANSIT (self)", statemachine.StatusInTransit, statemachine.StatusInTransit, false},
		{"OUT_FOR_DELIVERY to OUT_FOR_DELIVERY (self)", statemachine.StatusOutForDelivery, statemachine.StatusOutForDelivery, false},
		{"DELIVERED to DELIVERED (self)", statemachine.StatusDelivered, statemachine.StatusDelivered, false},
		{"FAILED to FAILED (self)", statemachine.StatusFailed, statemachine.StatusFailed, false},

		// Refused: out of terminal states
		{"DELIVERED to CREATED (terminal)", statemachine.StatusDelivered, statemachine.StatusCreated, false},
		{"DELIVERED to PICKED_UP (terminal)", statemachine.StatusDelivered, statemachine.StatusPickedUp, false},
		{"DELIVERED to IN_TRANSIT (terminal)", statemachine.StatusDelivered, statemachine.StatusInTransit, false},
		{"DELIVERED to OUT_FOR_DELIVERY (terminal)", statemachine.StatusDelivered, statemachine.StatusOutForDelivery, false},
		{"DELIVERED to FAILED (terminal)", statemachine.StatusDelivered, statemachine.StatusFailed, false},
		{"FAILED to CREATED (terminal)", statemachine.StatusFailed, statemachine.StatusCreated, false},
		{"FAILED to PICKED_UP (terminal)", statemachine.StatusFailed, statemachine.StatusPickedUp, false},
		{"FAILED to IN_TRANSIT (terminal)", statemachine.StatusFailed, statemachine.StatusInTransit, false},
		{"FAILED to OUT_FOR_DELIVERY (terminal)", statemachine.StatusFailed, statemachine.StatusOutForDelivery, false},
		{"FAILED to DELIVERED (terminal)", statemachine.StatusFailed, statemachine.StatusDelivered, false},

		// Unknown / invalid status
		{"UNKNOWN from status", statemachine.ParcelStatus("UNKNOWN"), statemachine.StatusPickedUp, false},
		{"UNKNOWN to status", statemachine.StatusCreated, statemachine.ParcelStatus("UNKNOWN"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := statemachine.CanTransition(tt.from, tt.to)
			if actual != tt.expected {
				t.Errorf("CanTransition(%s, %s) = %v; want %v", tt.from, tt.to, actual, tt.expected)
			}
		})
	}
}

func TestValidateTransition(t *testing.T) {
	tests := []struct {
		name         string
		from         statemachine.ParcelStatus
		to           statemachine.ParcelStatus
		input        statemachine.TransitionInput
		wantErrCode  string
		wantHTTPCode int
	}{
		// Valid transitions
		{
			name:        "Valid CREATED to PICKED_UP",
			from:        statemachine.StatusCreated,
			to:          statemachine.StatusPickedUp,
			input:       statemachine.TransitionInput{},
			wantErrCode: "",
		},
		{
			name:        "Valid PICKED_UP to IN_TRANSIT with optional note",
			from:        statemachine.StatusPickedUp,
			to:          statemachine.StatusInTransit,
			input:       statemachine.TransitionInput{Note: "Picked up at sorting center"},
			wantErrCode: "",
		},
		{
			name:        "Valid IN_TRANSIT to OUT_FOR_DELIVERY",
			from:        statemachine.StatusInTransit,
			to:          statemachine.StatusOutForDelivery,
			input:       statemachine.TransitionInput{},
			wantErrCode: "",
		},
		{
			name: "Valid OUT_FOR_DELIVERY to DELIVERED with recipient",
			from: statemachine.StatusOutForDelivery,
			to:   statemachine.StatusDelivered,
			input: statemachine.TransitionInput{
				RecipientName: "John Doe",
				Note:          "Handed directly to recipient",
			},
			wantErrCode: "",
		},
		{
			name: "Valid OUT_FOR_DELIVERY to FAILED with reason",
			from: statemachine.StatusOutForDelivery,
			to:   statemachine.StatusFailed,
			input: statemachine.TransitionInput{
				FailureReason: "Customer address unreachable after 3 attempts",
			},
			wantErrCode: "",
		},
		{
			name: "Valid CREATED to FAILED with reason",
			from: statemachine.StatusCreated,
			to:   statemachine.StatusFailed,
			input: statemachine.TransitionInput{
				FailureReason: "Package damaged before pickup",
			},
			wantErrCode: "",
		},

		// Disallowed transition -> 409 INVALID_TRANSITION
		{
			name:         "Disallowed transition CREATED to DELIVERED",
			from:         statemachine.StatusCreated,
			to:           statemachine.StatusDelivered,
			input:        statemachine.TransitionInput{RecipientName: "John Doe"},
			wantErrCode:  "INVALID_TRANSITION",
			wantHTTPCode: 409,
		},
		{
			name:         "Disallowed transition IN_TRANSIT to PICKED_UP",
			from:         statemachine.StatusInTransit,
			to:           statemachine.StatusPickedUp,
			input:        statemachine.TransitionInput{},
			wantErrCode:  "INVALID_TRANSITION",
			wantHTTPCode: 409,
		},
		{
			name:         "Disallowed transition DELIVERED to FAILED (terminal)",
			from:         statemachine.StatusDelivered,
			to:           statemachine.StatusFailed,
			input:        statemachine.TransitionInput{FailureReason: "Some reason here"},
			wantErrCode:  "INVALID_TRANSITION",
			wantHTTPCode: 409,
		},

		// DELIVERED missing recipient -> 422 RECIPIENT_REQUIRED
		{
			name:         "DELIVERED with empty recipient",
			from:         statemachine.StatusOutForDelivery,
			to:           statemachine.StatusDelivered,
			input:        statemachine.TransitionInput{RecipientName: ""},
			wantErrCode:  "RECIPIENT_REQUIRED",
			wantHTTPCode: 422,
		},
		{
			name:         "DELIVERED with whitespace-only recipient",
			from:         statemachine.StatusOutForDelivery,
			to:           statemachine.StatusDelivered,
			input:        statemachine.TransitionInput{RecipientName: "   \t\n  "},
			wantErrCode:  "RECIPIENT_REQUIRED",
			wantHTTPCode: 422,
		},
		{
			name: "DELIVERED with recipient exceeding 150 chars",
			from: statemachine.StatusOutForDelivery,
			to:   statemachine.StatusDelivered,
			input: statemachine.TransitionInput{
				RecipientName: strings.Repeat("A", 151),
			},
			wantErrCode:  "VALIDATION_ERROR",
			wantHTTPCode: 422,
		},

		// FAILED missing or invalid reason -> 422 FAILURE_REASON_REQUIRED
		{
			name:         "FAILED with empty reason",
			from:         statemachine.StatusOutForDelivery,
			to:           statemachine.StatusFailed,
			input:        statemachine.TransitionInput{FailureReason: ""},
			wantErrCode:  "FAILURE_REASON_REQUIRED",
			wantHTTPCode: 422,
		},
		{
			name:         "FAILED with whitespace-only reason",
			from:         statemachine.StatusOutForDelivery,
			to:           statemachine.StatusFailed,
			input:        statemachine.TransitionInput{FailureReason: "   "},
			wantErrCode:  "FAILURE_REASON_REQUIRED",
			wantHTTPCode: 422,
		},
		{
			name:         "FAILED with reason too short (< 5 chars)",
			from:         statemachine.StatusOutForDelivery,
			to:           statemachine.StatusFailed,
			input:        statemachine.TransitionInput{FailureReason: "Nope"},
			wantErrCode:  "FAILURE_REASON_REQUIRED",
			wantHTTPCode: 422,
		},
		{
			name: "FAILED with reason exactly 5 chars (valid min)",
			from: statemachine.StatusOutForDelivery,
			to:   statemachine.StatusFailed,
			input: statemachine.TransitionInput{
				FailureReason: "12345",
			},
			wantErrCode: "",
		},
		{
			name: "FAILED with reason exactly 255 chars (valid max)",
			from: statemachine.StatusOutForDelivery,
			to:   statemachine.StatusFailed,
			input: statemachine.TransitionInput{
				FailureReason: strings.Repeat("R", 255),
			},
			wantErrCode: "",
		},
		{
			name: "FAILED with reason too long (> 255 chars)",
			from: statemachine.StatusOutForDelivery,
			to:   statemachine.StatusFailed,
			input: statemachine.TransitionInput{
				FailureReason: strings.Repeat("R", 256),
			},
			wantErrCode:  "FAILURE_REASON_REQUIRED",
			wantHTTPCode: 422,
		},

		// Note length validation (> 500 chars -> 422 VALIDATION_ERROR)
		{
			name: "Note exceeding 500 characters",
			from: statemachine.StatusCreated,
			to:   statemachine.StatusPickedUp,
			input: statemachine.TransitionInput{
				Note: strings.Repeat("N", 501),
			},
			wantErrCode:  "VALIDATION_ERROR",
			wantHTTPCode: 422,
		},
		{
			name: "Note with exactly 500 characters (valid)",
			from: statemachine.StatusCreated,
			to:   statemachine.StatusPickedUp,
			input: statemachine.TransitionInput{
				Note: strings.Repeat("N", 500),
			},
			wantErrCode: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := statemachine.ValidateTransition(tt.from, tt.to, tt.input)
			if tt.wantErrCode == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}

			if err == nil {
				t.Fatalf("expected error code %s, got nil", tt.wantErrCode)
			}

			var domainErr *statemachine.DomainError
			if !errors.As(err, &domainErr) {
				t.Fatalf("expected *statemachine.DomainError, got %T (%v)", err, err)
			}

			if domainErr.Code != tt.wantErrCode {
				t.Errorf("error code = %s; want %s", domainErr.Code, tt.wantErrCode)
			}

			if tt.wantHTTPCode != 0 && domainErr.HTTPStatus != tt.wantHTTPCode {
				t.Errorf("HTTP status = %d; want %d", domainErr.HTTPStatus, tt.wantHTTPCode)
			}
		})
	}
}

func TestCanAssignCourier(t *testing.T) {
	tests := []struct {
		name         string
		status       statemachine.ParcelStatus
		wantErrCode  string
		wantHTTPCode int
	}{
		{"CREATED can be assigned", statemachine.StatusCreated, "", 0},
		{"PICKED_UP can be assigned", statemachine.StatusPickedUp, "", 0},
		{"IN_TRANSIT can be assigned", statemachine.StatusInTransit, "", 0},
		{"OUT_FOR_DELIVERY can be assigned", statemachine.StatusOutForDelivery, "", 0},
		{"DELIVERED cannot be assigned (PARCEL_CLOSED)", statemachine.StatusDelivered, "PARCEL_CLOSED", 409},
		{"FAILED cannot be assigned (PARCEL_CLOSED)", statemachine.StatusFailed, "PARCEL_CLOSED", 409},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := statemachine.CanAssignCourier(tt.status)
			if tt.wantErrCode == "" {
				if err != nil {
					t.Fatalf("expected nil error, got %v", err)
				}
				return
			}

			if err == nil {
				t.Fatalf("expected error code %s, got nil", tt.wantErrCode)
			}

			var domainErr *statemachine.DomainError
			if !errors.As(err, &domainErr) {
				t.Fatalf("expected *DomainError, got %T", err)
			}

			if domainErr.Code != tt.wantErrCode {
				t.Errorf("code = %s; want %s", domainErr.Code, tt.wantErrCode)
			}

			if domainErr.HTTPStatus != tt.wantHTTPCode {
				t.Errorf("HTTP status = %d; want %d", domainErr.HTTPStatus, tt.wantHTTPCode)
			}
		})
	}
}

func TestVerifyCourierOwnership(t *testing.T) {
	tests := []struct {
		name             string
		currentCourierID string
		actorCourierID   string
		wantErrCode      string
		wantHTTPCode     int
	}{
		{
			name:             "Matching courier ID is permitted",
			currentCourierID: "courier-uuid-1234",
			actorCourierID:   "courier-uuid-1234",
			wantErrCode:      "",
		},
		{
			name:             "Different courier ID returns 403 FORBIDDEN",
			currentCourierID: "courier-uuid-1234",
			actorCourierID:   "courier-uuid-9999",
			wantErrCode:      "FORBIDDEN",
			wantHTTPCode:     403,
		},
		{
			name:             "Empty current courier ID returns 403 FORBIDDEN",
			currentCourierID: "",
			actorCourierID:   "courier-uuid-1234",
			wantErrCode:      "FORBIDDEN",
			wantHTTPCode:     403,
		},
		{
			name:             "Empty actor courier ID returns 403 FORBIDDEN",
			currentCourierID: "courier-uuid-1234",
			actorCourierID:   "",
			wantErrCode:      "FORBIDDEN",
			wantHTTPCode:     403,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := statemachine.VerifyCourierOwnership(tt.currentCourierID, tt.actorCourierID)
			if tt.wantErrCode == "" {
				if err != nil {
					t.Fatalf("expected nil error, got %v", err)
				}
				return
			}

			if err == nil {
				t.Fatalf("expected error %s, got nil", tt.wantErrCode)
			}

			var domainErr *statemachine.DomainError
			if !errors.As(err, &domainErr) {
				t.Fatalf("expected *DomainError, got %T", err)
			}

			if domainErr.Code != tt.wantErrCode {
				t.Errorf("code = %s; want %s", domainErr.Code, tt.wantErrCode)
			}

			if domainErr.HTTPStatus != tt.wantHTTPCode {
				t.Errorf("HTTP status = %d; want %d", domainErr.HTTPStatus, tt.wantHTTPCode)
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
		wantErrCode      string
	}{
		{
			name:             "Valid courier update to IN_TRANSIT",
			currentCourierID: "c1",
			actorCourierID:   "c1",
			from:             statemachine.StatusPickedUp,
			to:               statemachine.StatusInTransit,
			input:            statemachine.TransitionInput{},
			wantErrCode:      "",
		},
		{
			name:             "Unassigned or different courier blocked before status check",
			currentCourierID: "c1",
			actorCourierID:   "c2",
			from:             statemachine.StatusPickedUp,
			to:               statemachine.StatusInTransit,
			input:            statemachine.TransitionInput{},
			wantErrCode:      "FORBIDDEN",
		},
		{
			name:             "Assigned courier with invalid transition",
			currentCourierID: "c1",
			actorCourierID:   "c1",
			from:             statemachine.StatusCreated,
			to:               statemachine.StatusDelivered,
			input:            statemachine.TransitionInput{RecipientName: "John"},
			wantErrCode:      "INVALID_TRANSITION",
		},
		{
			name:             "Assigned courier with missing recipient on DELIVERED",
			currentCourierID: "c1",
			actorCourierID:   "c1",
			from:             statemachine.StatusOutForDelivery,
			to:               statemachine.StatusDelivered,
			input:            statemachine.TransitionInput{},
			wantErrCode:      "RECIPIENT_REQUIRED",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := statemachine.ValidateCourierTransition(tt.currentCourierID, tt.actorCourierID, tt.from, tt.to, tt.input)
			if tt.wantErrCode == "" {
				if err != nil {
					t.Fatalf("expected nil error, got %v", err)
				}
				return
			}

			if err == nil {
				t.Fatalf("expected error %s, got nil", tt.wantErrCode)
			}

			var domainErr *statemachine.DomainError
			if !errors.As(err, &domainErr) {
				t.Fatalf("expected *DomainError, got %T", err)
			}

			if domainErr.Code != tt.wantErrCode {
				t.Errorf("code = %s; want %s", domainErr.Code, tt.wantErrCode)
			}
		})
	}
}

func TestStatusHelpers(t *testing.T) {
	if !statemachine.IsValidStatus(statemachine.StatusCreated) {
		t.Error("expected CREATED to be valid")
	}
	if statemachine.IsValidStatus("INVALID") {
		t.Error("expected INVALID to not be valid")
	}

	if !statemachine.IsTerminal(statemachine.StatusDelivered) {
		t.Error("expected DELIVERED to be terminal")
	}
	if !statemachine.IsTerminal(statemachine.StatusFailed) {
		t.Error("expected FAILED to be terminal")
	}
	if statemachine.IsTerminal(statemachine.StatusInTransit) {
		t.Error("expected IN_TRANSIT to not be terminal")
	}

	if !statemachine.IsClosed(statemachine.StatusDelivered) {
		t.Error("expected DELIVERED to be closed")
	}
	if !statemachine.IsClosed(statemachine.StatusFailed) {
		t.Error("expected FAILED to be closed")
	}
	if statemachine.IsClosed(statemachine.StatusOutForDelivery) {
		t.Error("expected OUT_FOR_DELIVERY to not be closed")
	}
}

