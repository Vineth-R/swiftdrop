package parcels

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func designParcel() Detail {
	description := "Phone case"
	return Detail{
		TrackingNumber: "SDK7M2Q9XP4",
		Status:         "IN_TRANSIT",
		Sender: Contact{
			Name: "Nimal Perera", Phone: "+94771234567",
			Address: "12 Temple Road", City: "Colombo",
		},
		Recipient: Contact{
			Name: "Amali Fernando", Phone: "+94712345678",
			Address: "45 Lake Drive", City: "Kandy",
		},
		WeightKG:     150,
		SizeCategory: "SMALL",
		Description:  &description,
		Fee: Fee{
			Currency: "LKR", BaseFee: 45000, WeightCharge: 10000,
			SizeSurcharge: 0, Total: 55000,
		},
		CreatedAt: time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 11, 4, 13, 0, 0, 0, time.UTC),
	}
}

func TestCustomerReceiptWithHistoryAndSupport(t *testing.T) {
	delivered := time.Date(2026, 11, 4, 15, 0, 0, 0, time.UTC)
	parcel := designParcel()
	parcel.DeliveredAt = &delivered
	history := []HistoryItem{
		{Status: "CREATED", ChangedAt: time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC)},
		{Status: "PICKED_UP", ChangedAt: time.Date(2026, 11, 4, 11, 0, 0, 0, time.UTC)},
		{Status: "IN_TRANSIT", ChangedAt: time.Date(2026, 11, 4, 13, 0, 0, 0, time.UTC)},
	}
	raw, err := ReceiptWithHistory(parcel, history, "support@swiftdrop.test")
	if err != nil {
		t.Fatalf("ReceiptWithHistory returned error: %v", err)
	}
	if !bytes.HasPrefix(raw, []byte("%PDF-")) || !bytes.Contains(raw, []byte("%%EOF")) {
		t.Fatal("customer receipt is not a complete PDF")
	}
	if dir := os.Getenv("RECEIPT_ARTIFACT_DIR"); dir != "" {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "customer-receipt.pdf"), raw, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, want := range []string{
		"SwiftDrop",
		"PARCEL DELIVERY RECEIPT",
		"SDK7M2Q9XP4",
		"DELIVERY FEE",
		"LKR 550.00",
		"STATUS HISTORY",
		"PICKED UP",
		"support@swiftdrop.test",
		"Page 1 of",
	} {
		if !bytes.Contains(raw, []byte(want)) {
			t.Fatalf("customer receipt does not contain %q", want)
		}
	}
	for _, forbidden := range []string{"ZoneID", "customer_user_id", "ownerUserId"} {
		if bytes.Contains(raw, []byte(forbidden)) {
			t.Fatalf("customer receipt exposed %q", forbidden)
		}
	}
}

func TestCustomerReceiptKeepsShapingForUnicodeValues(t *testing.T) {
	parcel := designParcel()
	parcel.Sender.Name = "නිමල් පෙරේරා"
	parcel.Recipient.Name = "அமலி பெர்னாண்டோ"
	raw, err := Receipt(parcel)
	if err != nil {
		t.Fatalf("Receipt returned error for Unicode values: %v", err)
	}
	for _, want := range []string{"/ToUnicode", "/FontFile2", "3 Tr", "f Q", "/ActualText"} {
		if !bytes.Contains(raw, []byte(want)) {
			t.Fatalf("customer receipt is missing shaped text component %q", want)
		}
	}
}

func TestReceiptDataByTrackingChecksOwnership(t *testing.T) {
	store := &memoryStore{}
	service := NewService(store, quote550())
	created, err := service.CreateCustomer(context.Background(), testUser, "", validInput())
	if err != nil {
		t.Fatalf("CreateCustomer returned error: %v", err)
	}
	if detail, err := service.ReceiptData(context.Background(), created.TrackingNumber, testUser, false); err != nil || detail.TrackingNumber != created.TrackingNumber {
		t.Fatalf("owner receipt data = %+v, %v", detail, err)
	}
	if _, err := service.ReceiptData(context.Background(), created.TrackingNumber, testOther, false); !errors.Is(err, ErrNotFound) {
		t.Fatalf("other customer receipt error = %v, want ErrNotFound", err)
	}
	for _, bad := range []string{"", "not-a-tracking", "SDK123", "SDX12345678"} {
		if _, err := service.ReceiptData(context.Background(), bad, testUser, false); !errors.Is(err, ErrNotFound) {
			t.Fatalf("invalid tracking %q error = %v, want ErrNotFound", bad, err)
		}
	}
}

func TestValidReceiptTracking(t *testing.T) {
	for _, tracking := range []string{"SDK7M2Q9XP4", "SDN4T8W2LC6"} {
		if !validReceiptTracking(tracking) {
			t.Fatalf("valid tracking %q was rejected", tracking)
		}
	}
	for _, tracking := range []string{"", "SDK7M2Q9XP", "SDK7M2Q9XP44", "SDX7M2Q9XP4", "SDK7M2Q9XP0", "SDK7M2Q9XPI"} {
		if validReceiptTracking(tracking) {
			t.Fatalf("invalid tracking %q was accepted", tracking)
		}
	}
}

func TestReceiptRowsDoNotLeakInternalIdentifiers(t *testing.T) {
	parcel := designParcel()
	parcel.ID = "6b1d2e3f-4a5b-4c6d-8e9f-0a1b2c3d4e5f"
	rows := customerRows(parcel, nil, nil)
	for _, row := range rows {
		if strings.Contains(row.text, parcel.ID) || strings.Contains(row.right, parcel.ID) {
			t.Fatal("customer receipt rows contain the internal parcel identifier")
		}
	}
}
