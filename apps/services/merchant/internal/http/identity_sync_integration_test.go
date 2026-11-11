//go:build integration

package servicehttp

import (
	"testing"

	"github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/repository"
)

// PendingIdentitySyncs tells the administrator's screen which rejected or
// suspended merchants still need their status to reach Identity.
func TestPendingIdentitySyncsListsOnlyStatusesIdentityHasNotApplied(t *testing.T) {
	ctx, _, repo := newWebhookTestPool(t)
	admin := userID(t)
	reason := "Repeated delivery complaints."

	newMerchant := func(t *testing.T) string {
		t.Helper()
		id := userID(t)
		if err := repo.CreateMerchant(ctx, repository.CreateMerchantParams{
			ID: id, OwnerUserID: userID(t), BusinessName: "Sync Test " + id[:8],
			BusinessRegNo: "SYNC-" + id[:8], ContactEmail: "sync@example.test",
			ContactPhone: "+94771234567", PickupAddress: "12 Galle Road", PickupZoneID: userID(t),
		}); err != nil {
			t.Fatal(err)
		}
		return id
	}
	approve := func(t *testing.T, id string) {
		t.Helper()
		_, transition, err := repo.BeginApprovalTransition(ctx, id, admin, userID(t))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := repo.ApplyApprovalTransition(ctx, id, admin, transition.ID); err != nil {
			t.Fatal(err)
		}
	}
	pending := func(t *testing.T, ids ...string) map[string]bool {
		t.Helper()
		got, err := repo.PendingIdentitySyncs(ctx, ids)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}

	t.Run("nobody asked for gives an empty answer", func(t *testing.T) {
		if got := pending(t); len(got) != 0 {
			t.Fatalf("pending = %v, want none", got)
		}
	})

	t.Run("a pending application is never listed", func(t *testing.T) {
		id := newMerchant(t)
		if got := pending(t, id); got[id] {
			t.Fatal("a PENDING merchant was listed")
		}
	})

	t.Run("a rejection is listed until Identity applies it", func(t *testing.T) {
		id := newMerchant(t)
		_, transition, err := repo.BeginRejectionTransition(ctx, id, admin, userID(t), &reason)
		if err != nil {
			t.Fatal(err)
		}
		if !pending(t, id)[id] {
			t.Fatal("a rejection that has not reached Identity was not listed")
		}
		if err := repo.MarkTransitionResult(ctx, transition.ID, false, "identity lifecycle failed"); err != nil {
			t.Fatal(err)
		}
		if !pending(t, id)[id] {
			t.Fatal("a FAILED sync was not listed")
		}
		if err := repo.MarkTransitionResult(ctx, transition.ID, true, ""); err != nil {
			t.Fatal(err)
		}
		if pending(t, id)[id] {
			t.Fatal("an applied rejection was still listed")
		}
	})

	t.Run("a suspension is listed, and a reapproval clears it", func(t *testing.T) {
		id := newMerchant(t)
		approve(t, id)
		if pending(t, id)[id] {
			t.Fatal("an approved merchant was listed")
		}
		if _, _, err := repo.BeginSuspensionTransition(ctx, id, admin, userID(t), &reason); err != nil {
			t.Fatal(err)
		}
		if !pending(t, id)[id] {
			t.Fatal("a suspension that has not reached Identity was not listed")
		}
		approve(t, id)
		if pending(t, id)[id] {
			t.Fatal("a reapproved merchant was still listed")
		}
	})

	t.Run("only the transition behind the current status counts", func(t *testing.T) {
		id := newMerchant(t)
		approve(t, id)
		// First suspension never reached Identity, then the merchant was approved
		// again and suspended a second time, and that one did reach it.
		if _, _, err := repo.BeginSuspensionTransition(ctx, id, admin, userID(t), &reason); err != nil {
			t.Fatal(err)
		}
		approve(t, id)
		_, second, err := repo.BeginSuspensionTransition(ctx, id, admin, userID(t), &reason)
		if err != nil {
			t.Fatal(err)
		}
		if err := repo.MarkTransitionResult(ctx, second.ID, true, ""); err != nil {
			t.Fatal(err)
		}
		if pending(t, id)[id] {
			t.Fatal("an older failed suspension counted after a later one was applied")
		}
	})

	t.Run("several merchants are answered at once", func(t *testing.T) {
		stuck, done, waiting := newMerchant(t), newMerchant(t), newMerchant(t)
		if _, _, err := repo.BeginRejectionTransition(ctx, stuck, admin, userID(t), nil); err != nil {
			t.Fatal(err)
		}
		_, applied, err := repo.BeginRejectionTransition(ctx, done, admin, userID(t), nil)
		if err != nil {
			t.Fatal(err)
		}
		if err := repo.MarkTransitionResult(ctx, applied.ID, true, ""); err != nil {
			t.Fatal(err)
		}
		got := pending(t, stuck, done, waiting)
		if !got[stuck] || got[done] || got[waiting] || len(got) != 1 {
			t.Fatalf("pending = %v, want only %s", got, stuck)
		}
	})
}
