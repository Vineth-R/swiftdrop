package parcels

import (
	"errors"
	"strings"
	"testing"
)

type nulField struct {
	name  string
	value func(*CreateInput) *string
}

func nulContactFields() []nulField {
	return []nulField{
		{"sender.name", func(in *CreateInput) *string { return &in.Sender.Name }},
		{"sender.phone", func(in *CreateInput) *string { return &in.Sender.Phone }},
		{"sender.address", func(in *CreateInput) *string { return &in.Sender.Address }},
		{"sender.city", func(in *CreateInput) *string { return &in.Sender.City }},
		{"recipient.name", func(in *CreateInput) *string { return &in.Recipient.Name }},
		{"recipient.phone", func(in *CreateInput) *string { return &in.Recipient.Phone }},
		{"recipient.address", func(in *CreateInput) *string { return &in.Recipient.Address }},
		{"recipient.city", func(in *CreateInput) *string { return &in.Recipient.City }},
		{"description", func(in *CreateInput) *string {
			if in.Description == nil {
				value := "Books"
				in.Description = &value
			}
			return in.Description
		}},
	}
}

func requireInvalidTextRejected(t *testing.T, err error, field string, store *memoryStore, quote *testQuoter) {
	t.Helper()
	var invalid *Invalid
	if !errors.As(err, &invalid) {
		t.Fatalf("invalid text returned %v, want validation error", err)
	}
	found := false
	for _, issue := range invalid.Issues {
		found = found || issue.Field == field
	}
	if !found {
		t.Fatalf("validation did not identify %s: %+v", field, invalid.Issues)
	}
	if quote.calls != 0 || store.calls != 0 || store.exists {
		t.Fatalf("invalid text reached pricing or persistence: pricing=%d writes=%d exists=%t", quote.calls, store.calls, store.exists)
	}
}

func TestCustomerNULRejectedBeforeDependencies(t *testing.T) {
	for _, field := range nulContactFields() {
		t.Run(field.name, func(t *testing.T) {
			input := validInput()
			value := field.value(&input)
			*value += "\x00"
			store, quote := &memoryStore{}, quote550()
			_, err := NewService(store, quote).CreateCustomer(t.Context(), testUser, testOther, input)
			requireInvalidTextRejected(t, err, field.name, store, quote)
		})
	}
}

func merchantValidationInput(in CreateInput) MerchantInput {
	return MerchantInput{MerchantID: testOther, CreatedByUserID: testUser, ExternalOrderID: "SHOP-100", IdempotencyKey: "merchant-key", Pickup: in.Sender, Recipient: in.Recipient, WeightKG: in.WeightKG, SizeCategory: in.SizeCategory, Description: in.Description}
}

func TestMerchantNULRejectedBeforeDependencies(t *testing.T) {
	for _, field := range nulContactFields() {
		t.Run(field.name, func(t *testing.T) {
			input := validInput()
			value := field.value(&input)
			*value += "\x00"
			store, quote := &memoryStore{}, quote550()
			_, err := NewService(store, quote).CreateMerchant(t.Context(), merchantValidationInput(input))
			requireInvalidTextRejected(t, err, field.name, store, quote)
		})
	}
	for _, field := range []string{"externalOrderId", "idempotencyKey"} {
		t.Run(field, func(t *testing.T) {
			input := merchantValidationInput(validInput())
			if field == "externalOrderId" {
				input.ExternalOrderID += "\x00"
			} else {
				input.IdempotencyKey += "\x00"
			}
			store, quote := &memoryStore{}, quote550()
			_, err := NewService(store, quote).CreateMerchant(t.Context(), input)
			requireInvalidTextRejected(t, err, field, store, quote)
		})
	}
}

func TestParcelTextPreservesUnicodeNewlinesAndTabs(t *testing.T) {
	input := validInput()
	input.Sender.Name = "සිංහල\tනම\nதமிழ்"
	input.Sender.Address = "ගෙදර\n12\tRoad"
	input.Sender.City = "கொழும்பு\tColombo"
	input.Recipient.Name = "தமிழ்\nபெயர்"
	input.Recipient.Address = "முகவரி\t45\nStreet"
	input.Recipient.City = "මහනුවර\tKandy"
	description := "සිංහල\nதமிழ்\tBooks"
	input.Description = &description
	store := &memoryStore{}
	created, err := NewService(store, quote550()).CreateCustomer(t.Context(), testUser, "", input)
	if err != nil || created.Sender != input.Sender || created.Recipient != input.Recipient || created.Description == nil || *created.Description != description {
		t.Fatalf("legitimate free text changed or rejected: %v", err)
	}
	merchant := merchantValidationInput(input)
	merchant.ExternalOrderID = "SHOP\tORDER\nසිංහල"
	merchant.IdempotencyKey = "merchant\tkey\nதமிழ்"
	merchantStore := &memoryStore{}
	_, err = NewService(merchantStore, quote550()).CreateMerchant(t.Context(), merchant)
	if err != nil || merchantStore.p.ExternalOrderID == nil || *merchantStore.p.ExternalOrderID != merchant.ExternalOrderID {
		t.Fatalf("legitimate merchant reference changed or rejected: %v", err)
	}
}

func TestParcelRawTextLengthsRejectedBeforeDependencies(t *testing.T) {
	for _, field := range []struct {
		name  string
		value func(*CreateInput) *string
		max   int
	}{
		{"sender.name", func(in *CreateInput) *string { return &in.Sender.Name }, 150},
		{"sender.address", func(in *CreateInput) *string { return &in.Sender.Address }, 255},
		{"sender.city", func(in *CreateInput) *string { return &in.Sender.City }, 100},
		{"recipient.name", func(in *CreateInput) *string { return &in.Recipient.Name }, 150},
		{"recipient.address", func(in *CreateInput) *string { return &in.Recipient.Address }, 255},
		{"recipient.city", func(in *CreateInput) *string { return &in.Recipient.City }, 100},
	} {
		for _, padding := range []string{"leading", "trailing"} {
			for _, merchant := range []bool{false, true} {
				owner := "customer"
				if merchant {
					owner = "merchant"
				}
				t.Run(owner+"/"+field.name+"/"+padding, func(t *testing.T) {
					input := validInput()
					value := strings.Repeat("é", field.max)
					if padding == "leading" {
						value = " " + value
					} else {
						value += " "
					}
					*field.value(&input) = value
					store, quote := &memoryStore{}, quote550()
					service := NewService(store, quote)
					var err error
					if merchant {
						_, err = service.CreateMerchant(t.Context(), merchantValidationInput(input))
					} else {
						_, err = service.CreateCustomer(t.Context(), testUser, testOther, input)
					}
					requireInvalidTextRejected(t, err, field.name, store, quote)
				})
			}
		}
	}
}

func TestParcelRawTextAtLimitsPreserved(t *testing.T) {
	input := validInput()
	input.Sender.Name = " " + strings.Repeat("é", 148) + " "
	input.Sender.Address = " " + strings.Repeat("é", 253) + " "
	input.Sender.City = " " + strings.Repeat("é", 98) + " "
	input.Recipient.Name = input.Sender.Name
	input.Recipient.Address = input.Sender.Address
	input.Recipient.City = input.Sender.City
	created, err := NewService(&memoryStore{}, quote550()).CreateCustomer(t.Context(), testUser, "", input)
	if err != nil || created.Sender != input.Sender || created.Recipient != input.Recipient {
		t.Fatalf("valid raw text limits rejected or modified: %v", err)
	}
}
