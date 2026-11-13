package repository

import (
	"testing"

	"github.com/Vineth-R/swiftdrop/packages/go-common/postgres"
)

// The idempotency lock borrows a dedicated connection from the service pool.
// Keep the shared pool satisfying that seam so order creation with an
// Idempotency-Key cannot regress to an internal error at runtime.
var _ connectionPool = (*postgres.Pool)(nil)

func TestNewRequiresDatabase(t *testing.T) {
	repository, err := New(nil)
	if err == nil {
		t.Fatal("New(nil) error = nil, want error")
	}
	if repository != nil {
		t.Fatalf("New(nil) repository = %#v, want nil", repository)
	}
}

func TestValidTransition(t *testing.T) {
	tests := []struct {
		name string
		from MerchantStatus
		to   MerchantStatus
		want bool
	}{
		{name: "pending approved", from: MerchantStatusPending, to: MerchantStatusApproved, want: true},
		{name: "suspended approved", from: MerchantStatusSuspended, to: MerchantStatusApproved, want: true},
		{name: "pending rejected", from: MerchantStatusPending, to: MerchantStatusRejected, want: true},
		{name: "approved suspended", from: MerchantStatusApproved, to: MerchantStatusSuspended, want: true},
		{name: "approved rejected", from: MerchantStatusApproved, to: MerchantStatusRejected},
		{name: "rejected approved", from: MerchantStatusRejected, to: MerchantStatusApproved},
		{name: "pending suspended", from: MerchantStatusPending, to: MerchantStatusSuspended},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := validTransition(test.from, test.to); got != test.want {
				t.Fatalf("validTransition(%q, %q) = %v, want %v", test.from, test.to, got, test.want)
			}
		})
	}
}
