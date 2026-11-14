//go:build integration

package servicehttp

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/repository"
	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
	"github.com/Vineth-R/swiftdrop/packages/go-common/httpx"
	"github.com/Vineth-R/swiftdrop/packages/go-common/secretbox"
	"github.com/Vineth-R/swiftdrop/packages/webhookdelivery"
)

// This test runs the real router, the real JWT middleware and the real
// repository against a disposable PostgreSQL database. It proves the webhook
// settings endpoints are fail closed and owner scoped.

const webhookAuthTestSecret = "webhook-auth-integration-secret-32-bytes!"
const webhookAuthTestCookie = "sd_access"

func TestWebhookRoutesAuthenticationAndOwnership(t *testing.T) {
	ctx, pool, repo := newWebhookTestPool(t)
	gin.SetMode(gin.TestMode)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	box, err := secretbox.NewKeyringFromHex("k1", strings.Repeat("ab", 32), nil)
	if err != nil {
		t.Fatalf("secretbox.NewFromHex returned error: %v", err)
	}
	router := httpx.NewRouter(logger)
	if err := RegisterWebhookRoutes(router, WebhookConfig{
		Repository:   repo,
		Cipher:       box,
		Sender:       webhookdelivery.NewSender(),
		AccessSecret: webhookAuthTestSecret,
		CookieName:   webhookAuthTestCookie,
		Logger:       logger,
	}); err != nil {
		t.Fatalf("RegisterWebhookRoutes returned error: %v", err)
	}

	manager, err := auth.NewTokenManager(webhookAuthTestSecret, time.Hour)
	if err != nil {
		t.Fatalf("NewTokenManager returned error: %v", err)
	}
	token := func(t *testing.T, userID string, role auth.Role) string {
		t.Helper()
		signed, _, err := manager.Issue(userID, role)
		if err != nil {
			t.Fatalf("Issue returned error: %v", err)
		}
		return signed
	}

	merchantID, ownerUserID := createOwnedMerchant(t, ctx, repo)
	if _, err := repo.ApproveMerchant(ctx, merchantID, userID(t)); err != nil {
		t.Fatalf("ApproveMerchant returned error: %v", err)
	}
	otherMerchantID, _ := createOwnedMerchant(t, ctx, repo)
	if _, err := repo.ApproveMerchant(ctx, otherMerchantID, userID(t)); err != nil {
		t.Fatalf("ApproveMerchant returned error: %v", err)
	}
	// Give the other merchant a configured webhook so a leak would be visible.
	if _, err := repo.SaveWebhook(ctx, repository.SaveWebhookParams{
		ID:                 userID(t),
		MerchantID:         otherMerchantID,
		URL:                "https://93.184.216.34/other",
		EventTypes:         []string{"parcel.status_changed"},
		IsActive:           true,
		CandidateSecretEnc: []byte("v1:other"),
	}); err != nil {
		t.Fatalf("SaveWebhook for other merchant returned error: %v", err)
	}

	call := func(t *testing.T, method, path, body, bearer string) (*httptest.ResponseRecorder, map[string]any) {
		t.Helper()
		var reader io.Reader
		if body != "" {
			reader = bytes.NewBufferString(body)
		}
		request := httptest.NewRequest(method, path, reader)
		if body != "" {
			request.Header.Set("Content-Type", "application/json")
		}
		if bearer != "" {
			request.Header.Set("Authorization", "Bearer "+bearer)
		}
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		var decoded map[string]any
		_ = json.Unmarshal(recorder.Body.Bytes(), &decoded)
		return recorder, decoded
	}

	t.Run("no token is rejected", func(t *testing.T) {
		recorder, body := call(t, http.MethodGet, "/merchants/me/webhook", "", "")
		if recorder.Code != http.StatusUnauthorized || errorCode(recorder.Body.Bytes()) != "UNAUTHENTICATED" {
			t.Fatalf("got %d %v, want 401 UNAUTHENTICATED", recorder.Code, body)
		}
	})

	t.Run("a customer token is forbidden", func(t *testing.T) {
		recorder, body := call(t, http.MethodGet, "/merchants/me/webhook", "", token(t, userID(t), auth.RoleCustomer))
		if recorder.Code != http.StatusForbidden || errorCode(recorder.Body.Bytes()) != "FORBIDDEN" {
			t.Fatalf("got %d %v, want 403 FORBIDDEN", recorder.Code, body)
		}
	})

	t.Run("a merchant user without a merchant record is forbidden", func(t *testing.T) {
		recorder, body := call(t, http.MethodGet, "/merchants/me/webhook", "", token(t, userID(t), auth.RoleMerchant))
		if recorder.Code != http.StatusForbidden || errorCode(recorder.Body.Bytes()) != "FORBIDDEN" {
			t.Fatalf("got %d %v, want 403 FORBIDDEN", recorder.Code, body)
		}
	})

	t.Run("an unapproved merchant cannot write", func(t *testing.T) {
		pendingID, pendingOwner := createOwnedMerchant(t, ctx, repo)
		_ = pendingID
		recorder, body := call(t, http.MethodPut, "/merchants/me/webhook",
			`{"url":"https://93.184.216.34/hook"}`, token(t, pendingOwner, auth.RoleMerchant))
		if recorder.Code != http.StatusForbidden || errorCode(recorder.Body.Bytes()) != "MERCHANT_NOT_APPROVED" {
			t.Fatalf("got %d %v, want 403 MERCHANT_NOT_APPROVED", recorder.Code, body)
		}
	})

	t.Run("an approved merchant saves and reads only its own settings", func(t *testing.T) {
		bearer := token(t, ownerUserID, auth.RoleMerchant)
		recorder, body := call(t, http.MethodGet, "/merchants/me/webhook", "", bearer)
		if recorder.Code != http.StatusNotFound || errorCode(recorder.Body.Bytes()) != "WEBHOOK_NOT_CONFIGURED" {
			t.Fatalf("before save: got %d %v, want 404 WEBHOOK_NOT_CONFIGURED", recorder.Code, body)
		}
		if recorder.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("Cache-Control = %q, want no-store", recorder.Header().Get("Cache-Control"))
		}

		recorder, body = call(t, http.MethodPut, "/merchants/me/webhook",
			`{"url":"https://93.184.216.34/hook"}`, bearer)
		if recorder.Code != http.StatusOK {
			t.Fatalf("save: got %d %v", recorder.Code, body)
		}
		data, _ := body["data"].(map[string]any)
		if secret, _ := data["signingSecret"].(string); !strings.HasPrefix(secret, "whsec_") {
			t.Fatalf("save response has no signing secret: %v", body)
		}

		recorder, body = call(t, http.MethodGet, "/merchants/me/webhook", "", bearer)
		if recorder.Code != http.StatusOK {
			t.Fatalf("after save: got %d %v", recorder.Code, body)
		}
		data, _ = body["data"].(map[string]any)
		if data["url"] != "https://93.184.216.34/hook" {
			t.Fatalf("GET returned %v", data)
		}
		if _, leaked := data["signingSecret"]; leaked {
			t.Fatal("GET leaked the signing secret")
		}

		stored, err := repo.WebhookByMerchant(ctx, merchantID)
		if err != nil {
			t.Fatalf("WebhookByMerchant returned error: %v", err)
		}
		if stored.URL != "https://93.184.216.34/hook" {
			t.Fatalf("stored URL = %q", stored.URL)
		}
		if stored.MerchantID != merchantID {
			t.Fatalf("stored webhook belongs to %q, want %q", stored.MerchantID, merchantID)
		}
	})

	t.Run("one merchant cannot see another merchant's delivery log", func(t *testing.T) {
		// Insert an attempt for the other merchant, then confirm this merchant's
		// log is still empty. The other merchant's webhook ID is not exposed.
		otherWebhook, err := repo.WebhookByMerchant(ctx, otherMerchantID)
		if err != nil {
			t.Fatalf("WebhookByMerchant returned error: %v", err)
		}
		status := 503
		insertAttempt(t, ctx, pool, otherWebhook.ID, otherMerchantID, "SDKOTHER01", 1, &status, false, false, time.Now().UTC())

		recorder, body := call(t, http.MethodGet, "/merchants/me/webhook/deliveries",
			"", token(t, ownerUserID, auth.RoleMerchant))
		if recorder.Code != http.StatusOK {
			t.Fatalf("got %d %v", recorder.Code, body)
		}
		rows, _ := body["data"].([]any)
		if len(rows) != 0 {
			t.Fatalf("this merchant saw %d attempts from another merchant", len(rows))
		}
	})
}
