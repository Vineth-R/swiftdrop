package receiptdesign

import (
	"testing"
	"time"
)

func TestMoneyAndDecimal(t *testing.T) {
	for _, test := range []struct {
		cents    int64
		currency string
		want     string
	}{
		{55000, "LKR", "LKR 550.00"},
		{5, "LKR", "LKR 0.05"},
		{-125, "LKR", "LKR -1.25"},
		{100, "", "1.00"},
	} {
		if got := Money(test.cents, test.currency); got != test.want {
			t.Fatalf("Money(%d, %q) = %q, want %q", test.cents, test.currency, got, test.want)
		}
	}
	if got := Decimal(150); got != "1.50" {
		t.Fatalf("Decimal(150) = %q", got)
	}
}

func TestReceiptLabelFormatting(t *testing.T) {
	if got := StatusLabel("out_for_delivery"); got != "OUT FOR DELIVERY" {
		t.Fatalf("StatusLabel = %q", got)
	}
	if got := JoinContact("", "Kandy", " "); got != "Kandy" {
		t.Fatalf("JoinContact = %q", got)
	}
	if got := JoinContact("", ""); got != "—" {
		t.Fatalf("empty JoinContact = %q", got)
	}
	if got := Fallback(" ", "none"); got != "none" {
		t.Fatalf("Fallback = %q", got)
	}
}

func TestDateTimeUseUTC(t *testing.T) {
	value := time.Date(2026, 11, 4, 10, 0, 0, 0, time.FixedZone("local", 5*3600))
	if got := DateTime(value); got != "04 Nov 2026 05:00 UTC" {
		t.Fatalf("DateTime = %q", got)
	}
	if got := Date(value); got != "04 Nov 2026" {
		t.Fatalf("Date = %q", got)
	}
}
