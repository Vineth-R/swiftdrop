// Package orders implements the merchant order snapshot, idempotent creation
// against the Parcel internal API, and the merchant-scoped reads.
package orders

import (
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/Vineth-R/swiftdrop/packages/go-common/validation"
)

var (
	// ErrNotFound hides another merchant's order as well as a missing one.
	ErrNotFound = errors.New("merchant order not found")
	// ErrDuplicateOrder means an existing order or idempotency key was reused
	// with a different normalized request.
	ErrDuplicateOrder = errors.New("duplicate merchant order")
	// ErrParcelUnavailable means Parcel Operations could not create the parcel.
	ErrParcelUnavailable = errors.New("parcel service unavailable")
	// ErrZoneUnavailable means the merchant's default pickup zone could not be read.
	ErrZoneUnavailable = errors.New("pickup zone unavailable")
	// ErrTrackingMismatch means an event does not belong to the stored parcel.
	ErrTrackingMismatch = errors.New("parcel status event does not match the merchant order")
)

// Invalid carries field-level validation issues that are safe to return.
type Invalid struct{ Issues []validation.Issue }

func (e *Invalid) Error() string { return "invalid merchant order request" }

// UpstreamInvalid carries a safe validation code and optional field issues from
// Parcel Operations.
type UpstreamInvalid struct {
	Code   string
	Issues []validation.Issue
}

func (e *UpstreamInvalid) Error() string { return "parcel rejected the order" }

// Decimal is an exact number of hundredths. Money and parcel weights never pass
// through float64.
type Decimal int64

func (d *Decimal) UnmarshalJSON(raw []byte) error {
	text := strings.TrimSpace(string(raw))
	if text == "" || len(text) > 32 {
		return errors.New("expected a decimal number")
	}
	if strings.ContainsAny(text, "eE") {
		return errors.New("exponents are not accepted")
	}
	parts := strings.SplitN(text, ".", 2)
	if len(parts) == 2 && len(parts[1]) > 2 {
		return errors.New("expected at most two decimal places")
	}
	r, ok := new(big.Rat).SetString(text)
	if !ok {
		return errors.New("invalid decimal")
	}
	r.Mul(r, big.NewRat(100, 1))
	if !r.IsInt() || !r.Num().IsInt64() {
		return errors.New("expected at most two decimal places")
	}
	n := r.Num().Int64()
	if n < -999999999999 || n > 999999999999 {
		return errors.New("decimal exceeds precision")
	}
	*d = Decimal(n)
	return nil
}

func (d Decimal) MarshalJSON() ([]byte, error) { return []byte(d.String()), nil }

func (d Decimal) String() string {
	n := int64(d)
	sign := ""
	if n < 0 {
		sign = "-"
		n = -n
	}
	return fmt.Sprintf("%s%d.%02d", sign, n/100, n%100)
}

func (d Decimal) Cents() int64 { return int64(d) }

// Contact is a pickup or recipient snapshot.
type Contact struct {
	Name    string `json:"name"`
	Phone   string `json:"phone"`
	Address string `json:"address"`
	City    string `json:"city"`
	ZoneID  string `json:"zoneId"`
}

// Fee is the immutable price snapshot returned by Parcel Operations.
type Fee struct {
	Currency      string  `json:"currency"`
	BaseFee       Decimal `json:"baseFee"`
	WeightCharge  Decimal `json:"weightCharge"`
	SizeSurcharge Decimal `json:"sizeSurcharge"`
	Total         Decimal `json:"total"`
}

// CreateInput is the API 8.16 request body.
type CreateInput struct {
	ExternalOrderID string   `json:"externalOrderId"`
	Pickup          *Contact `json:"pickup,omitempty"`
	Recipient       Contact  `json:"recipient"`
	WeightKG        Decimal  `json:"weightKg"`
	SizeCategory    string   `json:"sizeCategory"`
	Description     *string  `json:"description,omitempty"`
}

// Normalized is the validated request with the pickup resolved.
type Normalized struct {
	ExternalOrderID string
	IdempotencyKey  string
	Pickup          Contact
	Recipient       Contact
	WeightKG        Decimal
	SizeCategory    string
	Description     *string
}

// MerchantProfile is the authenticated merchant data the order service needs.
type MerchantProfile struct {
	ID            string
	OwnerUserID   string
	BusinessName  string
	ContactPhone  string
	PickupAddress string
	PickupZoneID  string
}

// Order is the persisted merchant order snapshot.
type Order struct {
	ID              string
	MerchantID      string
	ExternalOrderID string
	IdempotencyKey  *string
	RequestHash     string
	ParcelID        string
	TrackingNumber  string
	Pickup          Contact
	Recipient       Contact
	WeightKG        Decimal
	SizeCategory    string
	Description     *string
	Fee             Fee
	Status          string
	LastStatusAt    time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// TimelineEvent is one applied parcel status event.
type TimelineEvent struct {
	Status string    `json:"status"`
	At     time.Time `json:"at"`
}

// Detail is an order plus its ordered timeline.
type Detail struct {
	Order
	Timeline []TimelineEvent
}

// ListFilter is the API 8.17 query.
type ListFilter struct {
	MerchantID      string
	Status          string
	From            *time.Time
	To              *time.Time
	ExternalOrderID string
	Sort            string
	Desc            bool
	Page            int
	PageSize        int
}

// CreateParams is the transaction input for the repository.
type CreateParams struct {
	MerchantID      string
	ExternalOrderID string
	IdempotencyKey  *string
	RequestHash     string
	ParcelID        string
	TrackingNumber  string
	Pickup          Contact
	Recipient       Contact
	WeightKG        Decimal
	SizeCategory    string
	Description     *string
	Fee             Fee
	Status          string
}

// ParcelInput is sent to the trusted internal Parcel creation endpoint.
type ParcelInput struct {
	MerchantID      string
	CreatedByUserID string
	ExternalOrderID string
	IdempotencyKey  string
	Pickup          Contact
	Recipient       Contact
	WeightKG        Decimal
	SizeCategory    string
	Description     *string
}

// ParcelResult is the subset of the Parcel response the merchant needs.
type ParcelResult struct {
	ParcelID       string
	TrackingNumber string
	Status         string
	Fee            Fee
}

// StatusEvent is one validated parcel.status_changed.v1 event.
type StatusEvent struct {
	EventID        string
	MerchantID     string
	ParcelID       string
	TrackingNumber string
	PreviousStatus string
	Status         string
	OccurredAt     time.Time
	CorrelationID  string
}
