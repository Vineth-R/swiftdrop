//go:build integration

package servicehttp

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/repository"
	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
)

const adminSyncTestSecret = "admin-sync-test-secret-0123456789abcdef"

// The administrator's list and detail say which merchants still need their
// rejection or suspension to reach Identity, and leave the field out otherwise.
func TestAdminViewsReportAPendingIdentitySync(t *testing.T) {
	ctx, _, repo := newWebhookTestPool(t)
	router := NewWithOptions(Options{
		Repository: repo, AccessSecret: adminSyncTestSecret,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	manager, err := auth.NewTokenManager(adminSyncTestSecret, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	admin := userID(t)
	token, _, err := manager.Issue(admin, auth.RoleAdmin)
	if err != nil {
		t.Fatal(err)
	}
	get := func(t *testing.T, path string) (int, map[string]any) {
		t.Helper()
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.Header.Set("Authorization", "Bearer "+token)
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		var body map[string]any
		_ = json.Unmarshal(recorder.Body.Bytes(), &body)
		return recorder.Code, body
	}
	newMerchant := func(t *testing.T, name string) string {
		t.Helper()
		id := userID(t)
		if err := repo.CreateMerchant(ctx, repository.CreateMerchantParams{
			ID: id, OwnerUserID: userID(t), BusinessName: name + " " + id[:8],
			BusinessRegNo: "ADM-" + id[:8], ContactEmail: "admin-sync@example.test",
			ContactPhone: "+94771234567", PickupAddress: "12 Galle Road", PickupZoneID: userID(t),
		}); err != nil {
			t.Fatal(err)
		}
		return id
	}

	reason := "Repeated delivery complaints."
	stuck, applied, plain := newMerchant(t, "Stuck"), newMerchant(t, "Applied"), newMerchant(t, "Plain")
	if _, _, err := repo.BeginRejectionTransition(ctx, stuck, admin, userID(t), &reason); err != nil {
		t.Fatal(err)
	}
	_, done, err := repo.BeginRejectionTransition(ctx, applied, admin, userID(t), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.MarkTransitionResult(ctx, done.ID, true, ""); err != nil {
		t.Fatal(err)
	}

	t.Run("the list marks only the merchant whose sync is pending", func(t *testing.T) {
		code, body := get(t, "/admin/merchants?pageSize=100&q=")
		if code != http.StatusOK {
			t.Fatalf("status = %d, body = %v", code, body)
		}
		seen := map[string]any{}
		for _, item := range body["data"].([]any) {
			row := item.(map[string]any)
			if id, ok := row["id"].(string); ok {
				seen[id] = row["identitySync"]
			}
		}
		if seen[stuck] != "PENDING" {
			t.Fatalf("stuck merchant identitySync = %v, want PENDING", seen[stuck])
		}
		if seen[applied] != nil || seen[plain] != nil {
			t.Fatalf("identitySync leaked: applied = %v, plain = %v", seen[applied], seen[plain])
		}
	})

	t.Run("the detail says the same, and drops it once Identity has it", func(t *testing.T) {
		code, body := get(t, "/admin/merchants/"+stuck)
		data, _ := body["data"].(map[string]any)
		if code != http.StatusOK || data["identitySync"] != "PENDING" {
			t.Fatalf("status = %d, data = %v", code, data)
		}
		transition, err := repo.LatestTransition(ctx, stuck, repository.MerchantStatusRejected)
		if err != nil {
			t.Fatal(err)
		}
		if err := repo.MarkTransitionResult(ctx, transition.ID, true, ""); err != nil {
			t.Fatal(err)
		}
		_, body = get(t, "/admin/merchants/"+stuck)
		data, _ = body["data"].(map[string]any)
		if _, present := data["identitySync"]; present {
			t.Fatalf("identitySync stayed after the sync applied: %v", data)
		}
	})
}
