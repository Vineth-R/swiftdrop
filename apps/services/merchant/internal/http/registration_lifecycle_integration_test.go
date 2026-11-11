//go:build integration

package servicehttp

import (
	"strings"
	"testing"

	"github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/repository"
)

func TestRejectionReasonIsOptionalAndAudited(t *testing.T) {
	ctx, pool, repo := newWebhookTestPool(t)

	for _, test := range []struct {
		name   string
		reason *string
		want   *string
	}{
		{name: "without reason", reason: nil, want: nil},
		{name: "empty reason", reason: ptrTest("   "), want: nil},
		{name: "with reason", reason: ptrTest(" Documents could not be verified "), want: ptrTest("Documents could not be verified")},
	} {
		t.Run(test.name, func(t *testing.T) {
			merchantID := userID(t)
			if err := repo.CreateMerchant(ctx, repository.CreateMerchantParams{
				ID: merchantID, OwnerUserID: userID(t), BusinessName: "Reason Test " + merchantID[:8],
				BusinessRegNo: "REASON-" + merchantID[:8], ContactEmail: "reason@example.test",
				ContactPhone: "+94771234567", PickupAddress: "12 Galle Road", PickupZoneID: userID(t),
			}); err != nil {
				t.Fatal(err)
			}
			transitionID := userID(t)
			reason := test.reason
			if reason != nil {
				trimmed := strings.TrimSpace(*reason)
				if trimmed == "" {
					reason = nil
				} else {
					reason = &trimmed
				}
			}
			merchant, transition, err := repo.BeginRejectionTransition(ctx, merchantID, userID(t), transitionID, reason)
			if err != nil {
				t.Fatal(err)
			}
			if merchant.Status != repository.MerchantStatusRejected {
				t.Fatalf("status = %s, want REJECTED", merchant.Status)
			}
			var storedReason *string
			if err := pool.QueryRow(ctx, `SELECT rejection_reason FROM merchant.merchants WHERE id = $1`, merchantID).Scan(&storedReason); err != nil {
				t.Fatal(err)
			}
			if !sameOptional(storedReason, test.want) {
				t.Fatalf("stored reason = %v, want %v", storedReason, test.want)
			}
			var auditReason *string
			var provided bool
			if err := pool.QueryRow(ctx, `
				SELECT details->>'merchantReason', COALESCE((details->>'reasonProvided')::boolean, false)
				FROM merchant.audit_logs
				WHERE entity_id = $1 AND action = 'REJECTED'
				ORDER BY created_at DESC LIMIT 1`, merchantID,
			).Scan(&auditReason, &provided); err != nil {
				t.Fatal(err)
			}
			if !sameOptional(auditReason, test.want) || provided != (test.want != nil) {
				t.Fatalf("audit reason = %v provided=%v, want %v", auditReason, provided, test.want)
			}
			if transition.SyncStatus != "PENDING" {
				t.Fatalf("transition sync = %s, want PENDING", transition.SyncStatus)
			}
		})
	}
}

func ptrTest(value string) *string { return &value }

func sameOptional(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}
