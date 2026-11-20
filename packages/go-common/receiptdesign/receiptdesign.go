// Package receiptdesign holds the shared print identity for SwiftDrop PDF
// receipts: page geometry, the brand palette, and the date, money, decimal and
// contact formatting used by both customer and merchant receipts. It contains
// no business entities or PDF backend, so either renderer can use it without a
// dependency direction problem.
package receiptdesign

import (
	"fmt"
	"strings"
	"time"
)

// A4 geometry in millimetres. The custom customer renderer converts these to
// PDF points; the fpdf merchant renderer uses them directly.
const (
	PageWidthMM    = 210.0
	PageHeightMM   = 297.0
	MarginMM       = 14.0
	ContentWidthMM = PageWidthMM - 2*MarginMM
	HeaderHeightMM = 42.0
)

// RGB is one brand colour.
type RGB struct {
	R, G, B int
}

// The shared SwiftDrop print palette. Keep in step with the design tokens used
// by the web application.
var (
	Navy    = RGB{15, 23, 42}
	Ink     = RGB{30, 41, 59}
	Muted   = RGB{100, 116, 139}
	Line    = RGB{226, 232, 240}
	Surface = RGB{247, 249, 252}
	Lime    = RGB{198, 241, 53}
	White   = RGB{255, 255, 255}
	Pale    = RGB{190, 200, 215}
)

// Money formats an integer cent amount with the ISO currency code.
func Money(cents int64, currency string) string {
	sign := ""
	if cents < 0 {
		sign = "-"
		cents = -cents
	}
	value := fmt.Sprintf("%s%d.%02d", sign, cents/100, cents%100)
	if strings.TrimSpace(currency) == "" {
		return value
	}
	return currency + " " + value
}

// Decimal formats an integer hundredths value without a currency, for weights
// and other measured quantities.
func Decimal(cents int64) string {
	sign := ""
	if cents < 0 {
		sign = "-"
		cents = -cents
	}
	return fmt.Sprintf("%s%d.%02d", sign, cents/100, cents%100)
}

// Date is the short date used in receipt tables.
func Date(t time.Time) string { return t.UTC().Format("02 Jan 2006") }

// DateTime is the timestamp used in receipt headers and histories.
func DateTime(t time.Time) string { return t.UTC().Format("02 Jan 2006 15:04 UTC") }

// StatusLabel turns a machine status into a printable label.
func StatusLabel(status string) string {
	return strings.ReplaceAll(strings.ToUpper(strings.TrimSpace(status)), "_", " ")
}

// JoinContact joins the non-empty parts with the print separator. Callers pass
// a human value such as a name, phone or address and its city.
func JoinContact(parts ...string) string {
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if strings.TrimSpace(part) != "" {
			out = append(out, strings.TrimSpace(part))
		}
	}
	if len(out) == 0 {
		return "—"
	}
	return strings.Join(out, " · ")
}

// Fallback returns value, or substitute when value is blank.
func Fallback(value, substitute string) string {
	if strings.TrimSpace(value) == "" {
		return substitute
	}
	return value
}

// MMToPoints converts millimetres to PDF points.
func MMToPoints(mm float64) float64 { return mm * 72 / 25.4 }
