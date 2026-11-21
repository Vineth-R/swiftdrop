package pricing

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"strconv"
	"strings"

	"github.com/Vineth-R/swiftdrop/packages/go-common/validation"
)

var (
	ErrZoneNotFound        = errors.New("zone not found")
	ErrRouteNotServiceable = errors.New("route not serviceable")
	ErrWeightNotPriced     = errors.New("weight not priced")
	ErrNotFound            = errors.New("resource not found")
	ErrUnavailable         = errors.New("pricing database unavailable")
)

type Invalid struct{ Field, Issue string }

func (e *Invalid) Error() string { return "invalid " + e.Field + ": " + e.Issue }

// Decimal stores exact hundredths for NUMERIC(...,2) values and JSON numbers.
type Decimal int64

func ParseDecimal(raw string) (Decimal, error) {
	if len(raw) > 32 {
		return 0, errors.New("decimal is too long")
	}
	if !decimalPattern.MatchString(raw) {
		return 0, errors.New("invalid decimal")
	}
	if at := strings.IndexAny(raw, "eE"); at >= 0 {
		exponent, err := strconv.Atoi(raw[at+1:])
		if err != nil || exponent < -18 || exponent > 18 {
			return 0, errors.New("invalid decimal exponent")
		}
	}
	rational, ok := new(big.Rat).SetString(raw)
	if !ok {
		return 0, errors.New("invalid decimal")
	}
	rational.Mul(rational, big.NewRat(100, 1))
	if !rational.IsInt() || !rational.Num().IsInt64() {
		return 0, errors.New("decimal requires at most two fractional digits")
	}
	value := rational.Num().Int64()
	if value < -999999999999 || value > 999999999999 {
		return 0, errors.New("decimal exceeds database precision")
	}
	return Decimal(value), nil
}

func (d *Decimal) UnmarshalJSON(raw []byte) error {
	var number json.Number
	if len(raw) == 0 || raw[0] == '"' || string(raw) == "null" {
		return &Invalid{Field: "decimal", Issue: "expected a decimal number"}
	}
	if err := json.Unmarshal(raw, &number); err != nil {
		return err
	}
	value, err := ParseDecimal(string(number))
	if err != nil {
		return &Invalid{Field: "decimal", Issue: err.Error()}
	}
	*d = value
	return nil
}
func (d Decimal) String() string {
	value := int64(d)
	prefix := ""
	if value < 0 {
		prefix = "-"
		value = -value
	}
	return fmt.Sprintf("%s%d.%02d", prefix, value/100, value%100)
}
func (d Decimal) MarshalJSON() ([]byte, error) { return []byte(d.String()), nil }

var decimalPattern = regexp.MustCompile(`^-?(0|[1-9][0-9]*)(\.[0-9]+)?([eE][+-]?[0-9]+)?$`)

func ValidID(id string) bool { return validation.UUID(id) }
func ValidSize(size string) bool {
	return validation.OneOf(size, "SMALL", "MEDIUM", "LARGE", "OVERSIZED")
}

type Input struct {
	OriginZoneID      string  `json:"originZoneId"`
	DestinationZoneID string  `json:"destinationZoneId"`
	WeightKg          Decimal `json:"weightKg"`
	SizeCategory      string  `json:"sizeCategory"`
}

func (in Input) Validate() error {
	if !validation.UUID(in.OriginZoneID) {
		return &Invalid{Field: "originZoneId", Issue: "must be a UUID"}
	}
	if !validation.UUID(in.DestinationZoneID) {
		return &Invalid{Field: "destinationZoneId", Issue: "must be a UUID"}
	}
	if in.WeightKg <= 0 || in.WeightKg > 999999 {
		return &Invalid{Field: "weightKg", Issue: "must be positive and fit two decimal places"}
	}
	if !validation.OneOf(in.SizeCategory, "SMALL", "MEDIUM", "LARGE", "OVERSIZED") {
		return &Invalid{Field: "sizeCategory", Issue: "unsupported size category"}
	}
	return nil
}

type Quote struct {
	Input
	Currency      string  `json:"currency"`
	BaseFee       Decimal `json:"baseFee"`
	WeightCharge  Decimal `json:"weightCharge"`
	SizeSurcharge Decimal `json:"sizeSurcharge"`
	Total         Decimal `json:"total"`
}

func BaseFee(value Decimal) Decimal       { return value }
func WeightCharge(value Decimal) Decimal  { return value }
func SizeSurcharge(value Decimal) Decimal { return value }
func Calculate(in Input, base, weight, size Decimal) Quote {
	return Quote{Input: in, Currency: "LKR", BaseFee: BaseFee(base), WeightCharge: WeightCharge(weight),
		SizeSurcharge: SizeSurcharge(size), Total: base + weight + size}
}

type Zone struct {
	ID        string   `json:"id"`
	Code      string   `json:"code"`
	Name      string   `json:"name"`
	Districts []string `json:"districts"`
	IsActive  bool     `json:"isActive"`
}
