package repository

import (
	"context"
	"errors"
	"testing"
)

func TestCreateMerchantRejectsInvalidRegistrationNumberBeforeDatabase(t *testing.T) {
	repository := &Repository{}
	for _, value := range []string{"", "   ", "\t", "PV", "ABCDEF", "PV_123456"} {
		err := repository.CreateMerchant(context.Background(), CreateMerchantParams{BusinessRegNo: value})
		if !errors.Is(err, ErrInvalidRegistrationNumber) {
			t.Fatalf("CreateMerchant(%q) error = %v, want ErrInvalidRegistrationNumber", value, err)
		}
	}
}

func TestCreatePendingMerchantRejectsInvalidRegistrationNumberBeforeDatabase(t *testing.T) {
	repository := &Repository{}
	for _, value := range []string{"", "   ", "\t", "PV", "ABCDEF", "PV_123456"} {
		_, err := repository.CreatePendingMerchant(context.Background(), "attempt", CreateMerchantParams{BusinessRegNo: value})
		if !errors.Is(err, ErrInvalidRegistrationNumber) {
			t.Fatalf("CreatePendingMerchant(%q) error = %v, want ErrInvalidRegistrationNumber", value, err)
		}
	}
}
