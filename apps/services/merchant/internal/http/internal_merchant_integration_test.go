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

type merchantContactBody struct {
	Data struct {
		MerchantID   string `json:"merchantId"`
		OwnerUserID  string `json:"ownerUserId"`
		BusinessName string `json:"businessName"`
		ContactEmail string `json:"contactEmail"`
		Status       string `json:"status"`
	} `json:"data"`
}

func TestInternalMerchantContactResolvesStoredRecipient(t *testing.T) {
	ctx, _, repo := newWebhookTestPool(t)
	router := NewWithOptions(Options{
		Repository: repo, InternalToken: merchantContactTestToken,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	newMerchant := func(t *testing.T) (string, string) {
		t.Helper()
		merchantID, owner := userID(t), userID(t)
		if err := repo.CreateMerchant(ctx, repository.CreateMerchantParams{
			ID: merchantID, OwnerUserID: owner, BusinessName: "Contact Test " + merchantID[:8],
			BusinessRegNo: "CONTACT-" + merchantID[:8], ContactEmail: "contact-" + merchantID[:8] + "@example.test",
			ContactPhone: "+94771234567", PickupAddress: "12 Galle Road", PickupZoneID: userID(t),
		}); err != nil {
			t.Fatal(err)
		}
		return merchantID, owner
	}

	get := func(t *testing.T, merchantID, token, requestID string) (*httptest.ResponseRecorder, merchantContactBody) {
		t.Helper()
		recorder := merchantContactRequest(t, router, merchantID, token, requestID)
		var body merchantContactBody
		_ = json.Unmarshal(recorder.Body.Bytes(), &body)
		return recorder, body
	}

	t.Run("approved merchant returns the stored contact", func(t *testing.T) {
		merchantID, owner := newMerchant(t)
		admin := userID(t)
		_, transition, err := repo.BeginApprovalTransition(ctx, merchantID, admin, userID(t))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := repo.ApplyApprovalTransition(ctx, merchantID, admin, transition.ID); err != nil {
			t.Fatal(err)
		}

		recorder, body := get(t, merchantID, merchantContactTestToken, "req-1")
		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
		}
		if body.Data.MerchantID != merchantID || body.Data.OwnerUserID != owner ||
			body.Data.BusinessName != "Contact Test "+merchantID[:8] ||
			body.Data.ContactEmail != "contact-"+merchantID[:8]+"@example.test" ||
			body.Data.Status != "APPROVED" {
			t.Fatalf("body = %+v", body.Data)
		}
	})

	t.Run("pending merchant is reported as pending", func(t *testing.T) {
		merchantID, _ := newMerchant(t)
		recorder, body := get(t, merchantID, merchantContactTestToken, "req-2")
		if recorder.Code != http.StatusOK || body.Data.Status != "PENDING" {
			t.Fatalf("status = %d body = %+v", recorder.Code, body.Data)
		}
	})

	t.Run("unknown merchant is not found", func(t *testing.T) {
		recorder, _ := get(t, userID(t), merchantContactTestToken, "req-3")
		if recorder.Code != http.StatusNotFound {
			t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
		}
	})

	t.Run("token is checked first", func(t *testing.T) {
		merchantID, _ := newMerchant(t)
		recorder, _ := get(t, merchantID, "", "req-4")
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
		}
	})
}
