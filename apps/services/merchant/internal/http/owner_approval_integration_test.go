//go:build integration

package servicehttp

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/repository"
)

const approvalTestToken = "internal-token-for-tests"

type approvalBody struct {
	Data struct {
		MerchantID string  `json:"merchantId"`
		Status     string  `json:"status"`
		Reason     *string `json:"reason"`
	} `json:"data"`
}

// Identity reads this endpoint when a merchant owner signs in. The reason is the
// administrator's text for a rejection or a suspension and nothing else.
func TestInternalApprovalCarriesTheDecisionReason(t *testing.T) {
	ctx, _, repo := newWebhookTestPool(t)
	router := NewWithOptions(Options{
		Repository: repo, InternalToken: approvalTestToken,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	approval := func(t *testing.T, owner, token, requestID string) (*httptest.ResponseRecorder, approvalBody) {
		t.Helper()
		request := httptest.NewRequest(http.MethodGet, "/internal/merchants/owners/"+owner+"/approval", nil)
		if token != "" {
			request.Header.Set("X-Internal-Token", token)
		}
		if requestID != "" {
			request.Header.Set("X-Request-Id", requestID)
		}
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		var body approvalBody
		_ = json.Unmarshal(recorder.Body.Bytes(), &body)
		return recorder, body
	}
	newMerchant := func(t *testing.T) (merchantID, owner string) {
		t.Helper()
		merchantID, owner = userID(t), userID(t)
		if err := repo.CreateMerchant(ctx, repository.CreateMerchantParams{
			ID: merchantID, OwnerUserID: owner, BusinessName: "Approval Test " + merchantID[:8],
			BusinessRegNo: "APPROVAL-" + merchantID[:8], ContactEmail: "approval@example.test",
			ContactPhone: "+94771234567", PickupAddress: "12 Galle Road", PickupZoneID: userID(t),
		}); err != nil {
			t.Fatal(err)
		}
		return merchantID, owner
	}

	t.Run("a pending application has a status and no reason", func(t *testing.T) {
		_, owner := newMerchant(t)
		recorder, body := approval(t, owner, approvalTestToken, "req-1")
		if recorder.Code != http.StatusOK || body.Data.Status != "PENDING" || body.Data.Reason != nil {
			t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body)
		}
	})

	t.Run("a rejection carries its reason", func(t *testing.T) {
		merchantID, owner := newMerchant(t)
		reason := "The registration number could not be verified."
		if _, _, err := repo.BeginRejectionTransition(ctx, merchantID, userID(t), userID(t), &reason); err != nil {
			t.Fatal(err)
		}
		recorder, body := approval(t, owner, approvalTestToken, "req-2")
		if recorder.Code != http.StatusOK || body.Data.Status != "REJECTED" || body.Data.Reason == nil || *body.Data.Reason != reason {
			t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body)
		}
	})

	t.Run("a rejection without a reason sends none", func(t *testing.T) {
		merchantID, owner := newMerchant(t)
		if _, _, err := repo.BeginRejectionTransition(ctx, merchantID, userID(t), userID(t), nil); err != nil {
			t.Fatal(err)
		}
		recorder, body := approval(t, owner, approvalTestToken, "req-3")
		if recorder.Code != http.StatusOK || body.Data.Status != "REJECTED" || body.Data.Reason != nil {
			t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body)
		}
	})

	t.Run("a suspension carries its reason and an approval clears it", func(t *testing.T) {
		merchantID, owner := newMerchant(t)
		admin := userID(t)
		_, approve, err := repo.BeginApprovalTransition(ctx, merchantID, admin, userID(t))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := repo.ApplyApprovalTransition(ctx, merchantID, admin, approve.ID); err != nil {
			t.Fatal(err)
		}
		recorder, body := approval(t, owner, approvalTestToken, "req-4")
		if body.Data.Status != "APPROVED" || body.Data.Reason != nil {
			t.Fatalf("approved merchant: status = %d body = %s", recorder.Code, recorder.Body)
		}

		reason := "Repeated delivery complaints."
		if _, _, err := repo.BeginSuspensionTransition(ctx, merchantID, admin, userID(t), &reason); err != nil {
			t.Fatal(err)
		}
		recorder, body = approval(t, owner, approvalTestToken, "req-5")
		if body.Data.Status != "SUSPENDED" || body.Data.Reason == nil || *body.Data.Reason != reason {
			t.Fatalf("suspended merchant: status = %d body = %s", recorder.Code, recorder.Body)
		}

		// Reapproval clears the reason, so a later answer never repeats it.
		_, reapprove, err := repo.BeginApprovalTransition(ctx, merchantID, admin, userID(t))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := repo.ApplyApprovalTransition(ctx, merchantID, admin, reapprove.ID); err != nil {
			t.Fatal(err)
		}
		recorder, body = approval(t, owner, approvalTestToken, "req-6")
		if body.Data.Status != "APPROVED" || body.Data.Reason != nil {
			t.Fatalf("reapproved merchant: status = %d body = %s", recorder.Code, recorder.Body)
		}
	})

	t.Run("the internal token and a request ID are still required", func(t *testing.T) {
		_, owner := newMerchant(t)
		if recorder, _ := approval(t, owner, "wrong-token", "req-7"); recorder.Code != http.StatusUnauthorized {
			t.Fatalf("wrong token status = %d, want 401", recorder.Code)
		}
		if recorder, _ := approval(t, owner, approvalTestToken, ""); recorder.Code != http.StatusBadRequest {
			t.Fatalf("missing request ID status = %d, want 400", recorder.Code)
		}
	})
}
