// Package parcels implements customer and internal parcel operations.
package parcels

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Vineth-R/swiftdrop/packages/go-common/validation"
)

var (
	ErrUnavailable = errors.New("parcel dependency unavailable")
	ErrNotFound    = errors.New("parcel not found")
	ErrConflict    = errors.New("idempotency key reused with different request")
)

type Invalid struct{ Issues []validation.Issue }

func (e *Invalid) Error() string { return "invalid parcel request" }

type PricingError struct{ Code string }

func (e *PricingError) Error() string { return "pricing rejected parcel" }

type PriceChanged struct{ CurrentFee Fee }

func (e *PriceChanged) Error() string { return "parcel price changed" }

// Decimal is an exact number of hundredths. Money never passes through float64.
type Decimal int64

var decimalPattern = regexp.MustCompile(`^-?(0|[1-9][0-9]*)(\.[0-9]+)?([eE][+-]?[0-9]+)?$`)

func (d *Decimal) UnmarshalJSON(raw []byte) error {
	if len(raw) > 32 || !decimalPattern.Match(raw) {
		return errors.New("expected a decimal number")
	}
	if at := strings.IndexAny(string(raw), "eE"); at >= 0 {
		exponent, err := strconv.Atoi(string(raw)[at+1:])
		if err != nil || exponent < -18 || exponent > 18 {
			return errors.New("invalid decimal exponent")
		}
	}
	r, ok := new(big.Rat).SetString(string(raw))
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
func (d Decimal) String() string {
	n := int64(d)
	sign := ""
	if n < 0 {
		sign = "-"
		n = -n
	}
	return fmt.Sprintf("%s%d.%02d", sign, n/100, n%100)
}
func (d Decimal) MarshalJSON() ([]byte, error) { return []byte(d.String()), nil }

type Contact struct {
	Name    string `json:"name"`
	Phone   string `json:"phone"`
	Address string `json:"address"`
	City    string `json:"city"`
	ZoneID  string `json:"zoneId"`
}
type CreateInput struct {
	Sender       Contact  `json:"sender"`
	Recipient    Contact  `json:"recipient"`
	WeightKG     Decimal  `json:"weightKg"`
	SizeCategory string   `json:"sizeCategory"`
	Description  *string  `json:"description,omitempty"`
	ExpectedFee  *Decimal `json:"expectedFee,omitempty"`
}
type MerchantInput struct {
	MerchantID      string  `json:"merchantId"`
	CreatedByUserID string  `json:"createdByUserId"`
	ExternalOrderID string  `json:"externalOrderId"`
	IdempotencyKey  string  `json:"idempotencyKey"`
	Pickup          Contact `json:"pickup"`
	Recipient       Contact `json:"recipient"`
	WeightKG        Decimal `json:"weightKg"`
	SizeCategory    string  `json:"sizeCategory"`
	Description     *string `json:"description,omitempty"`
}
type Fee struct {
	Currency      string  `json:"currency"`
	BaseFee       Decimal `json:"baseFee"`
	WeightCharge  Decimal `json:"weightCharge"`
	SizeSurcharge Decimal `json:"sizeSurcharge"`
	Total         Decimal `json:"total"`
}
type Detail struct {
	ID             string     `json:"id"`
	TrackingNumber string     `json:"trackingNumber"`
	Status         string     `json:"status"`
	Sender         Contact    `json:"sender"`
	Recipient      Contact    `json:"recipient"`
	WeightKG       Decimal    `json:"weightKg"`
	SizeCategory   string     `json:"sizeCategory"`
	Description    *string    `json:"description"`
	Fee            Fee        `json:"fee"`
	Courier        any        `json:"courier"`
	FailureReason  *string    `json:"failureReason"`
	CreatedAt      time.Time  `json:"createdAt"`
	UpdatedAt      time.Time  `json:"updatedAt"`
	DeliveredAt    *time.Time `json:"deliveredAt"`
}
type ListItem struct {
	ID             string    `json:"id"`
	TrackingNumber string    `json:"trackingNumber"`
	Status         string    `json:"status"`
	RecipientName  string    `json:"recipientName"`
	DropoffCity    string    `json:"dropoffCity"`
	Total          Decimal   `json:"total"`
	Currency       string    `json:"currency"`
	CreatedAt      time.Time `json:"createdAt"`
}
type HistoryItem struct {
	PreviousStatus *string   `json:"previousStatus"`
	Status         string    `json:"status"`
	Note           *string   `json:"note"`
	FailureReason  *string   `json:"failureReason"`
	ActorRole      *string   `json:"actorRole"`
	ChangedAt      time.Time `json:"changedAt"`
}
type Summary struct {
	ParcelID        string  `json:"parcelId"`
	TrackingNumber  string  `json:"trackingNumber"`
	Status          string  `json:"status"`
	CustomerUserID  *string `json:"customerUserId"`
	MerchantID      *string `json:"merchantId"`
	ExternalOrderID *string `json:"externalOrderId"`
	RecipientName   string  `json:"recipientName"`
}
type Filter struct {
	Status         string
	From, To       *time.Time
	Page, PageSize int
	Sort           string
}

var phonePattern = regexp.MustCompile(`^\+[1-9][0-9]{8,14}$`)

func (in CreateInput) Validate(customer bool) error {
	v := validation.New()
	for _, c := range []struct {
		name  string
		value Contact
	}{{"sender", in.Sender}, {"recipient", in.Recipient}} {
		for _, field := range []struct {
			name  string
			value string
		}{{"name", c.value.Name}, {"phone", c.value.Phone}, {"address", c.value.Address}, {"city", c.value.City}} {
			v.Check(c.name+"."+field.name, !strings.ContainsRune(field.value, '\x00'), "must not contain NUL characters")
		}
		// Enforce limits on the raw text that will be persisted, while requiring
		// non-whitespace content. Trimming only for the limit can exceed varchar.
		v.Required(c.name+".name", c.value.Name)
		v.MaxLength(c.name+".name", c.value.Name, 150, "must be at most 150 characters")
		v.Check(c.name+".phone", phonePattern.MatchString(c.value.Phone), "must be a valid phone number")
		v.Required(c.name+".address", c.value.Address)
		v.MaxLength(c.name+".address", c.value.Address, 255, "must be at most 255 characters")
		v.Required(c.name+".city", c.value.City)
		v.MaxLength(c.name+".city", c.value.City, 100, "must be at most 100 characters")
		v.UUID(c.name+".zoneId", c.value.ZoneID)
	}
	v.Check("weightKg", in.WeightKG > 0 && in.WeightKG <= 999999, "must be positive and at most 9999.99")
	v.Enum("sizeCategory", in.SizeCategory, "must be SMALL, MEDIUM, LARGE or OVERSIZED", "SMALL", "MEDIUM", "LARGE", "OVERSIZED")
	if in.Description != nil {
		v.Check("description", !strings.ContainsRune(*in.Description, '\x00'), "must not contain NUL characters")
		v.MaxLength("description", *in.Description, 255, "must be at most 255 characters")
	}
	if customer {
		v.Check("expectedFee", in.ExpectedFee != nil && *in.ExpectedFee >= 0, "must be a nonnegative fee")
	}
	if !v.Empty() {
		return &Invalid{v.Issues()}
	}
	return nil
}
func invalid(field, issue string) error {
	return &Invalid{[]validation.Issue{{Field: field, Issue: issue}}}
}
func validStatus(s string) bool {
	return validation.OneOf(s, "CREATED", "PICKED_UP", "IN_TRANSIT", "OUT_FOR_DELIVERY", "DELIVERED", "FAILED")
}

type breakdown struct {
	BaseFee       Decimal `json:"base_fee"`
	WeightCharge  Decimal `json:"weight_charge"`
	SizeSurcharge Decimal `json:"size_surcharge"`
}

func storedFee(raw json.RawMessage, currency string, total Decimal) (Fee, error) {
	var b breakdown
	if err := json.Unmarshal(raw, &b); err != nil {
		return Fee{}, err
	}
	if b.BaseFee+b.WeightCharge+b.SizeSurcharge != total {
		return Fee{}, errors.New("stored fee is inconsistent")
	}
	return Fee{currency, b.BaseFee, b.WeightCharge, b.SizeSurcharge, total}, nil
}
