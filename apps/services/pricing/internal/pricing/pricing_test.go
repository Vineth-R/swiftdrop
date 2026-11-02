package pricing

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestDecimalExactness(t *testing.T) {
	for _, test := range []struct {
		name, raw, want string
		invalid         bool
	}{
		{name: "fraction", raw: "0.10", want: "0.10"},
		{name: "exponent", raw: "1e-2", want: "0.01"},
		{name: "trailing precision", raw: "1.2300", want: "1.23"},
		{name: "database maximum", raw: "9999999999.99", want: "9999999999.99"},
		{name: "over precision", raw: "0.001", invalid: true},
		{name: "overflow", raw: "10000000000", invalid: true},
		{name: "exponent attack", raw: "1e999999999", invalid: true},
		{name: "non decimal fraction", raw: "1/2", invalid: true},
		{name: "not a number", raw: "NaN", invalid: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			value, err := ParseDecimal(test.raw)
			if test.invalid {
				if err == nil {
					t.Fatal("accepted invalid decimal")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			raw, err := json.Marshal(value)
			if err != nil || string(raw) != test.want {
				t.Fatalf("encoded %s, error %v; want %s", raw, err, test.want)
			}
		})
	}
}
func TestInputValidation(t *testing.T) {
	valid := Input{OriginZoneID: "11111111-1111-4111-8111-111111111111", DestinationZoneID: "22222222-2222-4222-8222-222222222222", WeightKg: 200, SizeCategory: "SMALL"}
	for _, test := range []struct {
		name    string
		change  func(*Input)
		invalid bool
	}{
		{name: "valid", change: func(*Input) {}},
		{name: "zero weight", change: func(in *Input) { in.WeightKg = 0 }, invalid: true},
		{name: "negative weight", change: func(in *Input) { in.WeightKg = -1 }, invalid: true},
		{name: "size", change: func(in *Input) { in.SizeCategory = "XL" }, invalid: true},
		{name: "zone format", change: func(in *Input) { in.OriginZoneID = "not-a-uuid" }, invalid: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			in := valid
			test.change(&in)
			err := in.Validate()
			var invalid *Invalid
			if test.invalid != errors.As(err, &invalid) {
				t.Fatalf("unexpected validation result %v", err)
			}
		})
	}
}
func TestCalculateSizes(t *testing.T) {
	for _, test := range []struct {
		name  string
		size  Decimal
		total Decimal
	}{
		{name: "SMALL", size: 0, total: 55000}, {name: "MEDIUM", size: 15000, total: 70000},
		{name: "LARGE", size: 40000, total: 95000}, {name: "OVERSIZED", size: 90000, total: 145000},
	} {
		t.Run(test.name, func(t *testing.T) {
			quote := Calculate(Input{SizeCategory: test.name}, 45000, 10000, test.size)
			if quote.Total != test.total || quote.Currency != "LKR" || quote.BaseFee != 45000 || quote.WeightCharge != 10000 {
				t.Fatalf("unexpected quote %+v", quote)
			}
		})
	}
}
