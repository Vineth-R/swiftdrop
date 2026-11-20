package receipt

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/orders"
)

func testInput() Input {
	description := "Phone case"
	return Input{
		Merchant: Merchant{
			BusinessName:  "Demo Merchant",
			ContactEmail:  "merchant@example.test",
			ContactPhone:  "+94770000000",
			PickupAddress: "1 Demo Street, Colombo",
		},
		Order: orders.Detail{
			Order: orders.Order{
				ExternalOrderID: "SZ-100245",
				TrackingNumber:  "SDN4T8W2LC6",
				Pickup: orders.Contact{
					Name: "Demo Merchant", Phone: "+94770000000",
					Address: "1 Demo Street", City: "Colombo",
				},
				Recipient: orders.Contact{
					Name: "Amali Fernando", Phone: "+94712345678",
					Address: "45 Lake Drive", City: "Kandy",
				},
				WeightKG:     150,
				SizeCategory: "SMALL",
				Description:  &description,
				Fee: orders.Fee{
					Currency: "LKR", BaseFee: 45000, WeightCharge: 10000,
					SizeSurcharge: 0, Total: 55000,
				},
				Status:    "IN_TRANSIT",
				CreatedAt: time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC),
				UpdatedAt: time.Date(2026, 11, 4, 13, 0, 0, 0, time.UTC),
			},
			Timeline: []orders.TimelineEvent{
				{Status: "CREATED", At: time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC)},
				{Status: "PICKED_UP", At: time.Date(2026, 11, 4, 11, 0, 0, 0, time.UTC)},
				{Status: "IN_TRANSIT", At: time.Date(2026, 11, 4, 13, 0, 0, 0, time.UTC)},
			},
		},
		SupportEmail: "support@swiftdrop.test",
		GeneratedAt:  time.Date(2026, 11, 4, 14, 0, 0, 0, time.UTC),
	}
}

func TestRenderProducesPortablePDF(t *testing.T) {
	raw, err := Render(testInput())
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}
	if !bytes.HasPrefix(raw, []byte("%PDF-")) {
		t.Fatalf("receipt does not start with a PDF signature: %q", raw[:min(8, len(raw))])
	}
	for _, want := range []string{
		"SwiftDrop",
		"MERCHANT ORDER RECEIPT",
		"SDN4T8W2LC6",
		"DELIVERY FEE",
		"support@swiftdrop.test",
	} {
		if !bytes.Contains(raw, []byte(want)) {
			t.Fatalf("receipt does not contain %q", want)
		}
	}
	if bytes.Contains(raw, []byte("ownerUserId")) {
		t.Fatal("receipt exposed an internal owner identifier")
	}
	if !bytes.Contains(raw, []byte("SwiftDrop | merchant order receipt")) {
		t.Fatal("receipt footer does not use the ASCII separator")
	}
}

func TestRenderHandlesUnicodeNames(t *testing.T) {
	input := testInput()
	input.Order.Recipient.Name = "නිමල් පෙරේරා"
	input.Order.Recipient.Address = "45 වීදිය"
	raw, err := Render(input)
	if err != nil {
		t.Fatalf("Render returned error for a Sinhala name: %v", err)
	}
	if !bytes.HasPrefix(raw, []byte("%PDF-")) {
		t.Fatal("Unicode receipt is not a PDF")
	}
}

func TestRenderRejectsMissingFontsOnlyThroughError(t *testing.T) {
	// Render must return a complete PDF or an error, never a partial buffer.
	raw, err := Render(testInput())
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}
	if !bytes.Contains(raw, []byte("%%EOF")) {
		t.Fatal("receipt PDF is truncated")
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func TestLongFieldsStayWithinReasonablePageCount(t *testing.T) {
	input := testInput()
	input.Merchant.BusinessName = strings.Repeat("Long Merchant Business Name ", 10)
	input.Merchant.PickupAddress = strings.Repeat("123 Long Pickup Road ", 14)
	input.Order.ExternalOrderID = strings.Repeat("EXT-", 20)
	input.Order.Recipient.Name = strings.Repeat("Long Recipient Name ", 12)
	input.Order.Recipient.Address = strings.Repeat("987 Long Delivery Road ", 14)
	description := strings.Repeat("A long parcel description. ", 20)
	input.Order.Description = &description
	for i := 0; i < 30; i++ {
		input.Order.Timeline = append(input.Order.Timeline, orders.TimelineEvent{Status: "IN_TRANSIT", At: input.Order.CreatedAt.Add(time.Duration(i) * time.Hour)})
	}
	raw, err := Render(input)
	if err != nil {
		t.Fatalf("Render returned error for long fields: %v", err)
	}
	if !bytes.HasPrefix(raw, []byte("%PDF-")) || !bytes.Contains(raw, []byte("%%EOF")) {
		t.Fatal("long merchant receipt is not a complete PDF")
	}
	if pages := bytes.Count(raw, []byte("/Type /Page ")); pages > 3 {
		t.Fatalf("long merchant receipt used %d pages, want at most 3", pages)
	}
}
