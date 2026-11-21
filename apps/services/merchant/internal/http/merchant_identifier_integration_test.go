//go:build integration

package servicehttp

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMalformedMerchantIdentifiersReturnNotFound(t *testing.T) {
	_, _, repo := newWebhookTestPool(t)
	router := NewWithOptions(Options{
		Repository: repo, AccessSecret: adminSyncTestSecret, InternalToken: "internal-token",
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	admin := adminToken(t, adminSyncTestSecret)

	for _, test := range []struct {
		name   string
		method string
		path   string
	}{
		{"admin detail", http.MethodGet, "/admin/merchants/not-a-uuid"},
		{"approve", http.MethodPost, "/admin/merchants/not-a-uuid/approve"},
		{"reject", http.MethodPost, "/admin/merchants/not-a-uuid/reject"},
		{"suspend", http.MethodPost, "/admin/merchants/not-a-uuid/suspend"},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(test.method, test.path, nil)
			request.Header.Set("Authorization", "Bearer "+admin)
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusNotFound || errorCode(recorder.Body.Bytes()) != "NOT_FOUND" {
				t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.Bytes())
			}
		})
	}

	t.Run("a valid but missing merchant is still not found", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodGet, "/admin/merchants/"+userID(t), nil)
		request.Header.Set("Authorization", "Bearer "+admin)
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusNotFound || errorCode(recorder.Body.Bytes()) != "NOT_FOUND" {
			t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.Bytes())
		}
	})

	for _, test := range []struct {
		name string
		id   string
	}{
		{"malformed internal approval owner", "not-a-uuid"},
		{"missing internal approval owner", userID(t)},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/internal/merchants/owners/"+test.id+"/approval", nil)
			request.Header.Set("X-Internal-Token", "internal-token")
			request.Header.Set("X-Request-Id", "test-request-id")
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusNotFound || errorCode(recorder.Body.Bytes()) != "NOT_FOUND" {
				t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.Bytes())
			}
		})
	}
}

func TestMalformedAPIKeyRevocationReturnsNotFound(t *testing.T) {
	ctx, repo, _, router, _ := newOrderHTTPFixture(t)
	merchantID, ownerID := createHTTPMerchant(t, ctx, repo)
	approve(t, ctx, repo, merchantID)
	token := merchantToken(t, ownerID)

	for _, test := range []struct {
		name string
		id   string
	}{
		{"malformed key id", "not-a-uuid"},
		{"missing key id", userID(t)},
	} {
		t.Run(test.name, func(t *testing.T) {
			status, body := requestJSON(t, router, http.MethodDelete, "/merchants/me/keys/"+test.id, nil,
				map[string]string{"Authorization": "Bearer " + token})
			if status != http.StatusNotFound || errorCode(body) != "NOT_FOUND" {
				t.Fatalf("status = %d body = %s", status, body)
			}
		})
	}

	key := newKey(t, ctx, repo, merchantID, "revoke")
	status, body := requestJSON(t, router, http.MethodDelete, "/merchants/me/keys/"+key.id, nil,
		map[string]string{"Authorization": "Bearer " + token})
	if status != http.StatusNoContent {
		t.Fatalf("revoke status = %d (%s)", status, body)
	}
	status, body = requestJSON(t, router, http.MethodDelete, "/merchants/me/keys/"+key.id, nil,
		map[string]string{"Authorization": "Bearer " + token})
	if status != http.StatusNotFound || errorCode(body) != "NOT_FOUND" {
		t.Fatalf("second revoke status = %d (%s)", status, body)
	}
}
